// SPDX-License-Identifier: AGPL-3.0-only

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
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

func postPush(t *testing.T, tsURL, token string, body any) (*http.Response, server.PushResult) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, tsURL+"/push", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	var out server.PushResult
	if resp.StatusCode == http.StatusOK {
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&out); err != nil {
			t.Fatalf("decode push response: %v", err)
		}
	}
	return resp, out
}

func TestPushUnauthorized(t *testing.T) {
	ts := newTestServer(t)
	resp, _ := postPush(t, ts.URL, "", map[string]string{"query": "fts5 syntax error"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestPushNonPostReturns405(t *testing.T) {
	ts := newTestServer(t)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		req, err := http.NewRequest(method, ts.URL+"/push", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s /push status = %d, want 405", method, resp.StatusCode)
		}
	}
}

func TestPushReturnsTrapCardsOnStrongMatch(t *testing.T) {
	ts := newTestServer(t)
	resp, out := postPush(t, ts.URL, token, map[string]string{
		"query": `FTS5: syntax error near "."`,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if out.Count < 1 {
		t.Fatalf("count = %d, want at least 1 hit", out.Count)
	}
	if out.Context == "" {
		t.Fatal("context must carry rendered trap cards")
	}
	for _, want := range []string{
		"TYPE: trap-cards",
		"TRUST: validated",
		"--- BEGIN EXPERIENCE DATA ---",
		"exp-0001",
		"Title:",
		"Applies to:",
		"The trap:",
		"The escape:",
		"modernc.org/sqlite",
		"Never hand raw text to MATCH",
	} {
		if !strings.Contains(out.Context, want) {
			t.Errorf("context missing %q:\n%s", want, out.Context)
		}
	}
	if countRealEndDelimiters(out.Context) != 1 {
		t.Fatalf("want exactly one end delimiter in context, got %d", countRealEndDelimiters(out.Context))
	}
	// The response exposes the injected ids so a client can close the feedback
	// loop (confirm_helpful / report_outcome on a pushed card).
	if len(out.IDs) == 0 || !contains(out.IDs, "exp-0001") {
		t.Errorf("response must list injected ids including exp-0001, got %v", out.IDs)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestPushWeakMatchReturnsEmpty(t *testing.T) {
	ts := newTestServerWith(t, mkServerRecord(t, 10,
		"Postgres HNSW index build is slow under tiny maintenance_work_mem",
		"building an hnsw vector index takes hours when maintenance_work_mem is small"))
	resp, out := postPush(t, ts.URL, token, map[string]string{"query": "index"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if out.Count != 0 || out.Context != "" {
		t.Errorf("weak near-miss must inject nothing, got count=%d context=%q", out.Count, out.Context)
	}
}

func TestPushNeverReturnsQuarantined(t *testing.T) {
	const sig = "quarantined-push-channel-sig-unique-6611"
	rec := mkInjectionRecord(t, 66, "quarantined")
	rec.Symptom.ErrorSignatures = []string{sig}
	ts := newTestServerWith(t, rec)

	resp, out := postPush(t, ts.URL, token, map[string]string{"query": sig})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if out.Count != 0 || out.Context != "" {
		t.Errorf("quarantined record must never be pushed, got count=%d context=%q", out.Count, out.Context)
	}
}

// ADR-0001 §4: the push hot path is embedding-free even when the server has an
// embedder configured for the pull channel.
func TestPushIgnoresDenseRetrieval(t *testing.T) {
	emb := &stubEmbedder{dim: 3, basis: map[string][]float32{
		"quux":  {1, 0, 0},
		"florb": {1, 0, 0},
	}}
	recs := []*record.Record{
		mkDensePushRecord(t, 1, "Connection handling", "the florb subsystem stalls under load"),
	}
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, testRepo); err != nil {
		t.Fatal(err)
	}
	if err := ix.EmbedCorpus(context.Background(), recs, emb); err != nil {
		t.Fatal(err)
	}
	h, err := server.New(server.Config{Index: ix, Token: token, Repo: testRepo, Embedder: emb})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptestNew(t, h)

	// Pull channel finds it via dense fusion.
	session := connect(t, ts)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_experience",
		Arguments: map[string]any{"query": "quux"},
	})
	if err != nil {
		t.Fatalf("search_experience: %v", err)
	}
	if res.IsError {
		t.Fatalf("search tool error: %s", toolText(res))
	}
	sb, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(sb), "exp-0001") {
		t.Fatalf("pull channel should surface dense hit, got %s", sb)
	}

	// Push channel must not — embedding-free only.
	resp, out := postPush(t, ts.URL, token, map[string]string{"query": "quux"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if out.Count != 0 || out.Context != "" {
		t.Errorf("push must not use dense retrieval, got count=%d context=%q", out.Count, out.Context)
	}
}

func TestPushRejectsEmptyQuery(t *testing.T) {
	ts := newTestServer(t)
	resp, _ := postPush(t, ts.URL, token, map[string]string{"query": "   "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestPushCapsAtThreeCards(t *testing.T) {
	var recs []*record.Record
	for i := range 5 {
		n := 20 + i
		recs = append(recs, mkServerRecord(t, n,
			"Shared lexical topic about widget calibration drift",
			"widget calibration drifts when the shared lexical topic appears"))
	}
	ts := newTestServerWith(t, recs...)
	resp, out := postPush(t, ts.URL, token, map[string]string{
		"query": "widget calibration drifts shared lexical topic",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if out.Count > 3 {
		t.Errorf("count = %d, want hard cap k≤3", out.Count)
	}
}

// stubEmbedder is a minimal dense stub for server-level push vs pull tests.
type stubEmbedder struct {
	dim   int
	basis map[string][]float32
}

func (s *stubEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	v := make([]float32, s.dim)
	low := strings.ToLower(text)
	for sub, b := range s.basis {
		if strings.Contains(low, sub) {
			for i := range v {
				v[i] += b[i]
			}
		}
	}
	return v, nil
}

func mkDensePushRecord(t *testing.T, num int, title, summary string) *record.Record {
	t.Helper()
	return mkServerRecord(t, num, title, summary)
}

func httptestNew(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}
