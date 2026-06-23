// SPDX-License-Identifier: AGPL-3.0-only

package server_test

import (
	"bytes"
	"context"
	"database/sql"
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
	"github.com/dotts-h/twiceshy/internal/testcorpus"
)

// A single served record that fails to load/parse (e.g. corpus drift between the
// FTS index and the records table) must not 500 the whole hot-path injection: the
// bad card is dropped and the rest are served ("empty is a valid answer").
func TestPushDegradesOnBadServedRecord(t *testing.T) {
	recs, err := record.LoadCorpus(testcorpus.Root())
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "ix.db")
	ix, err := index.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, testRepo); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	// Corrupt the stored markdown of a served record so record.Parse fails at push
	// time, while it stays in the FTS index (still served): real corpus drift.
	raw, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := raw.Exec("UPDATE records SET raw=? WHERE id=?", "not valid frontmatter", "exp-0001"); err != nil {
		t.Fatalf("corrupt record: %v", err)
	}
	_ = raw.Close()

	h, err := server.New(server.Config{Index: ix, Token: token, Repo: testRepo})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	resp, out := postPush(t, ts.URL, token, map[string]string{"query": `FTS5: syntax error near "."`})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (one bad card must not 500 the push)", resp.StatusCode)
	}
	if contains(out.IDs, "exp-0001") {
		t.Errorf("the unparseable served card must be dropped, got IDs %v", out.IDs)
	}
}

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

// TestPushCapsAtThreeCards exercises the k≤3 hard cap (ADR-0001 §3, index.MaxK)
// through the path that CAN serve more than 3 candidates: the fingerprint-exact
// bypass. Five validated records share the SAME error signature; querying that
// exact signature makes fingerprint.Generic hash identically for all five, so the
// gate is bypassed and all five are fingerprint-exact candidates — yet Retrieve
// must clamp the served set to exactly MaxK. Asserting == 3 (not just "> 3 false")
// proves the cap actually clamped 5 down to 3 rather than passing vacuously on an
// all-floored, zero-hit corpus.
func TestPushCapsAtThreeCards(t *testing.T) {
	const sharedSig = "panic: widget calibration drift exploded near offset 7"
	var recs []*record.Record
	for i := range 5 {
		n := 20 + i
		rec := mkServerRecord(t, n,
			"Shared lexical topic about widget calibration drift",
			"widget calibration drifts when the shared lexical topic appears")
		rec.Symptom.ErrorSignatures = []string{sharedSig}
		recs = append(recs, rec)
	}
	ts := newTestServerWith(t, recs...)
	// The query text must equal the indexed signature so fingerprint.Generic
	// hashes identically and the bypass fires for all five candidates.
	resp, out := postPush(t, ts.URL, token, map[string]string{
		"query": sharedSig,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if out.Count != 3 {
		t.Errorf("count = %d, want exactly 3 (k≤3 cap clamping 5 fingerprint-exact candidates, ADR-0001 §3)", out.Count)
	}
	if len(out.IDs) != 3 {
		t.Errorf("len(IDs) = %d, want 3 distinct served ids under the cap; got %v", len(out.IDs), out.IDs)
	}
	// The served ids must be a subset of the seeded five, with no duplicates —
	// the cap drops candidates, it does not invent or repeat them.
	seeded := map[string]bool{}
	for i := range 5 {
		seeded[fmt.Sprintf("exp-%04d", 20+i)] = true
	}
	seen := map[string]bool{}
	for _, id := range out.IDs {
		if !seeded[id] {
			t.Errorf("served id %q is not one of the seeded fingerprint-exact records", id)
		}
		if seen[id] {
			t.Errorf("served id %q appears more than once under the cap", id)
		}
		seen[id] = true
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
