package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/server"
)

const token = "s3cret-test-token"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	recs, err := record.LoadCorpus("../..")
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, "github.com/dotts-h/twiceshy"); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	h, err := server.New(server.Config{Index: ix, Token: token})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(req)
}

func connect(t *testing.T, ts *httptest.Server) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "twiceshy-test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{token}},
	}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestNewRequiresIndexAndToken(t *testing.T) {
	if _, err := server.New(server.Config{Index: nil, Token: "x"}); err == nil {
		t.Error("nil index must be rejected")
	}
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ix.Close() }()
	if _, err := server.New(server.Config{Index: ix, Token: ""}); err == nil {
		t.Error("empty bearer token must be rejected")
	}
}

func TestBearerAuthRejectsBadCredentials(t *testing.T) {
	ts := newTestServer(t)
	for name, header := range map[string]string{
		"no auth":      "",
		"wrong token":  "Bearer wrong",
		"not bearer":   "Basic " + token,
		"empty bearer": "Bearer ",
	} {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", resp.StatusCode)
			}
		})
	}
}

// Guarding test for exp-0003: the pull channel is MCP over streamable
// HTTP — a real SDK client must complete the handshake against the
// handler and see both Phase 1 tools.
func TestServerSpeaksStreamableHTTP(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range tools.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"search_experience", "get_experience"} {
		if !got[want] {
			t.Errorf("missing tool %q (have %v)", want, got)
		}
	}
}

func TestSearchExperienceTool(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_experience",
		Arguments: map[string]any{"query": `FTS5: syntax error near "."`},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", toolText(res))
	}
	out, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"exp-0001", "fingerprint"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("structured output missing %q: %s", want, out)
		}
	}
}

func TestSearchExperienceRejectsEmptyQuery(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_experience",
		Arguments: map[string]any{"query": "   "},
	})
	if err == nil && !res.IsError {
		t.Error("blank query must be a tool error")
	}
}

func TestGetExperienceTool(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_experience",
		Arguments: map[string]any{"id": "exp-0001"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", toolText(res))
	}
	out, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"exp-0001", "The trap"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("structured output missing %q", want)
		}
	}
}

func TestGetExperienceUnknownIDIsToolError(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_experience",
		Arguments: map[string]any{"id": "exp-9999"},
	})
	if err == nil && !res.IsError {
		t.Error("unknown id must be a tool error, not a silent success")
	}
}

// record_experience (Phase 3 write path) must be exposed alongside the pull tools.
func TestRecordExperienceListed(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "record_experience" {
			found = true
		}
	}
	if !found {
		t.Error("record_experience tool must be registered")
	}
}

// A new (non-duplicate) lesson is accepted as a QUARANTINED draft with an
// allocated id — never written validated; git/PR is the trust boundary.
// (It may classify novel OR similar depending on incidental lexical overlap;
// either way it must be quarantined, not validated.)
func TestRecordExperienceQuarantinesNewDraft(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "record_experience",
		Arguments: map[string]any{
			"kind":             "trap",
			"title":            "Connection pool dries up under a novel burst pattern",
			"summary":          "the pool runs dry under a previously-unseen condition",
			"error_signatures": []string{"snowflake-novel-signature-zzz-9182"},
			"ecosystem":        "Go",
			"package":          "example.com/db",
			"root_cause":       "leaked connections on a retry path",
			"fix":              "defer rows.Close on the retry branch",
			"guarding_test":    "TestPoolRetryGuard",
			"body":             "How the pool runs dry on retries and how to guard it.",
			"author":           "claude",
			"session":          "sess-test",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", toolText(res))
	}
	out, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"record_id":"exp-`) {
		t.Errorf("a recorded draft must get an allocated id: %s", s)
	}
	if !strings.Contains(s, "status: quarantined") {
		t.Errorf("the draft must be quarantined: %s", s)
	}
	if strings.Contains(s, "status: validated") || strings.Contains(s, "validated_at: 2") {
		t.Errorf("a recorded draft must never be born validated: %s", s)
	}
}

// An exact duplicate (matching an existing record's signature) is NOT recorded.
func TestRecordExperienceKnownNotDuplicated(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "record_experience",
		Arguments: map[string]any{
			"kind":             "trap",
			"title":            "Re-reporting the FTS5 punctuation syntax trap",
			"summary":          "fts5 match errors on punctuation",
			"error_signatures": []string{`FTS5: Syntax Error near "."`},
			"ecosystem":        "MCP",
			"root_cause":       "raw user text reached MATCH",
			"fix":              "quote tokens",
			"guarding_test":    "TestX",
			"body":             "duplicate of an existing trap.",
			"author":           "claude",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool error: %s", toolText(res))
	}
	s, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(s), "known") || !strings.Contains(string(s), "exp-0001") {
		t.Errorf("exact duplicate must be reported known with the existing id: %s", s)
	}
}

// A malformed draft (trap without a resolution) is a tool error, not a silent quarantine.
func TestRecordExperienceInvalidIsError(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "record_experience",
		Arguments: map[string]any{
			"kind":             "trap",
			"title":            "Incomplete trap missing its resolution block",
			"summary":          "something broke",
			"error_signatures": []string{"another-unique-sig-7766"},
			"body":             "no resolution provided.",
			"author":           "claude",
		},
	})
	if err == nil && !res.IsError {
		t.Error("invalid draft must be a tool error, not a silent quarantined record")
	}
}

// newTestServerWith builds a server over a caller-supplied synthetic corpus,
// for tests that need to control retrieval scores deterministically.
func newTestServerWith(t *testing.T, recs ...*record.Record) *httptest.Server {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, "github.com/dotts-h/twiceshy"); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	h, err := server.New(server.Config{Index: ix, Token: token})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

func mkServerRecord(t *testing.T, num int, title, summary string) *record.Record {
	t.Helper()
	src := fmt.Sprintf(`---
schema_version: 1
id: exp-%04d
kind: trap
status: validated
title: %q
symptom:
  summary: %q
applies_to:
  - ecosystem: PyPI
    package: pgvector
resolution: { root_cause: "a cause", fix: "a fix" }
guard: { repro: null, guarding_test: "TestThing" }
provenance:
  source: { author: "horia", session: null, pr: null }
  recorded_at: 2026-06-12
  validated_at: 2026-06-12
  valid: { from: 2026-06-12, until: null }
  superseded_by: null
---

Narrative.
`, num, title, summary)
	rec, err := record.Parse(fmt.Sprintf("experience/2026/%04d-rec.md", num), []byte(src))
	if err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}
	return rec
}

// ADR-0007: the pull channel (search_experience) applies the relevance floor.
// A query whose only match falls below DefaultFloor returns nothing — "empty is
// an answer", never a near-miss — while a genuine multi-term match still comes
// back. Before the fix the handler called the floor-free Search and leaked the
// weak near-miss.
func TestSearchExperienceFloorsNearMiss(t *testing.T) {
	ts := newTestServerWith(t, mkServerRecord(t, 10,
		"Postgres HNSW index build is slow under tiny maintenance_work_mem",
		"building an hnsw vector index takes hours when maintenance_work_mem is small"))
	session := connect(t, ts)

	weak, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_experience",
		Arguments: map[string]any{"query": "index"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if weak.IsError {
		t.Fatalf("tool error: %s", toolText(weak))
	}
	wb, _ := json.Marshal(weak.StructuredContent)
	if strings.Contains(string(wb), "exp-0010") {
		t.Errorf("weak single-token near-miss must be floored, got %s", wb)
	}

	strong, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_experience",
		Arguments: map[string]any{"query": "hnsw index build slow maintenance_work_mem vector"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	sb, _ := json.Marshal(strong.StructuredContent)
	if !strings.Contains(string(sb), "exp-0010") {
		t.Errorf("a genuine multi-term match must still return, got %s", sb)
	}
}

func toolText(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
