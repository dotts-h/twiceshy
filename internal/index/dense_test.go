// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// stubEmbedder maps any registered substring present in the text to a basis
// vector and sums them — deterministic, no network. Two different words can map
// to the SAME basis so a query and a record that share no lexical tokens still
// embed near each other (the case dense must catch and BM25 cannot).
type stubEmbedder struct {
	dim   int
	basis map[string][]float32
	calls int
}

func (s *stubEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	s.calls++
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

type errEmbedder struct{ calls int }

func (e *errEmbedder) Embed(context.Context, string) ([]float32, error) {
	e.calls++
	return nil, context.DeadlineExceeded
}

func embedAll(t *testing.T, ix *index.Index, recs []*record.Record, emb index.Embedder) {
	t.Helper()
	if err := ix.EmbedCorpus(context.Background(), recs, emb); err != nil {
		t.Fatalf("EmbedCorpus: %v", err)
	}
}

// Dense surfaces a record that shares NO lexical token with the query, via a
// matching embedding — something the embedding-free Search cannot do.
func TestRetrieveFusedDenseSurfacesLexicalMiss(t *testing.T) {
	// "quux" (query) and "florb" (record) map to the same basis vector.
	emb := &stubEmbedder{dim: 3, basis: map[string][]float32{
		"quux":  {1, 0, 0},
		"florb": {1, 0, 0},
	}}
	recs := []*record.Record{
		mkRecord(t, 1, "Connection handling", "the florb subsystem stalls under load", nil, "Go", ""),
		mkRecord(t, 2, "Unrelated topic", "totally different words here", nil, "Go", ""),
	}
	ix := openIndex(t, recs)
	embedAll(t, ix, recs, emb)

	q := index.Query{Text: "quux"} // no lexical overlap with any record
	plain, err := ix.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(plain) != 0 {
		t.Fatalf("embedding-free Search should miss; got %d hits", len(plain))
	}
	fused, err := ix.RetrieveFused(context.Background(), q, emb)
	if err != nil {
		t.Fatalf("RetrieveFused: %v", err)
	}
	if len(fused) != 1 || fused[0].ID != "exp-0001" {
		t.Fatalf("dense should surface exp-0001 via cosine; got %+v", fused)
	}
	if fused[0].Matched != index.MatchedDense {
		t.Errorf("matched = %q, want %q", fused[0].Matched, index.MatchedDense)
	}
}

// A fingerprint-exact hit keeps absolute precedence under fusion.
func TestRetrieveFusedFingerprintStillWins(t *testing.T) {
	sig := "panic: runtime error: index out of range [5]"
	emb := &stubEmbedder{dim: 3, basis: map[string][]float32{"florb": {1, 0, 0}}}
	recs := []*record.Record{
		mkRecord(t, 1, "Slice bounds trap", "guarding against OOB", []string{sig}, "Go", ""),
		mkRecord(t, 2, "Florb topic", "the florb thing", nil, "Go", ""),
	}
	ix := openIndex(t, recs)
	embedAll(t, ix, recs, emb)

	// Query is the exact signature (fingerprint hit on exp-0001) and also embeds
	// near exp-0002 (contains no "florb" → zero vec → below floor, so only the
	// fingerprint hit qualifies).
	fused, err := ix.RetrieveFused(context.Background(), index.Query{Text: sig}, emb)
	if err != nil {
		t.Fatalf("RetrieveFused: %v", err)
	}
	if len(fused) == 0 || fused[0].ID != "exp-0001" || fused[0].Matched != index.MatchedFingerprint {
		t.Fatalf("fingerprint hit must rank first; got %+v", fused)
	}
}

// Cap at MaxK survives fusion.
func TestRetrieveFusedCapsAtMaxK(t *testing.T) {
	basis := map[string][]float32{"shared": {1, 0, 0}}
	var recs []*record.Record
	for i := 1; i <= 6; i++ {
		recs = append(recs, mkRecord(t, i, "shared record", "the shared concept appears", nil, "Go", ""))
	}
	emb := &stubEmbedder{dim: 3, basis: basis}
	ix := openIndex(t, recs)
	embedAll(t, ix, recs, emb)

	fused, err := ix.RetrieveFused(context.Background(), index.Query{Text: "shared"}, emb)
	if err != nil {
		t.Fatalf("RetrieveFused: %v", err)
	}
	if len(fused) > index.MaxK {
		t.Errorf("returned %d hits, want <= %d", len(fused), index.MaxK)
	}
}

// With no embedder, RetrieveFused is exactly the embedding-free Retrieve.
func TestRetrieveFusedNilEmbedderFallsBackToSearch(t *testing.T) {
	recs := []*record.Record{
		mkRecord(t, 1, "FTS5 MATCH trap", "raw user input breaks the query", []string{"fts5: syntax error"}, "MCP", ""),
		mkRecord(t, 2, "Other topic here", "unrelated", nil, "Go", ""),
	}
	ix := openIndex(t, recs)

	q := index.Query{Text: "fts5 syntax error"}
	want, err := ix.Retrieve(context.Background(), q)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	got, err := ix.RetrieveFused(context.Background(), q, nil)
	if err != nil {
		t.Fatalf("RetrieveFused(nil): %v", err)
	}
	if !sameHits(want, got) {
		t.Errorf("nil-embedder fused != Retrieve\n want %+v\n got %+v", want, got)
	}
}

// An embedder that errors degrades gracefully to the embedding-free path.
func TestRetrieveFusedEmbedErrorFallsBack(t *testing.T) {
	recs := []*record.Record{
		mkRecord(t, 1, "FTS5 MATCH trap", "raw user input breaks the query", []string{"fts5: syntax error"}, "MCP", ""),
	}
	ix := openIndex(t, recs)
	q := index.Query{Text: "fts5 syntax error"}
	want, _ := ix.Retrieve(context.Background(), q)
	bad := &errEmbedder{}
	got, err := ix.RetrieveFused(context.Background(), q, bad)
	if err != nil {
		t.Fatalf("RetrieveFused must not error on embed failure: %v", err)
	}
	if bad.calls == 0 {
		t.Error("expected the embedder to be attempted")
	}
	if !sameHits(want, got) {
		t.Errorf("error-fallback != Retrieve\n want %+v\n got %+v", want, got)
	}
}

// Hot-path purity: Assess (push/ingest classification) NEVER embeds; only the
// pull path (RetrieveFused) does.
func TestAssessNeverEmbeds(t *testing.T) {
	emb := &stubEmbedder{dim: 3, basis: map[string][]float32{"quux": {1, 0, 0}, "florb": {1, 0, 0}}}
	recs := []*record.Record{mkRecord(t, 1, "Florb record", "the florb thing", nil, "Go", "")}
	ix := openIndex(t, recs)
	embedAll(t, ix, recs, emb) // build-time embedding is expected
	emb.calls = 0              // measure query-time only

	if _, err := ix.Assess(context.Background(), index.Query{Text: "quux"}); err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if emb.calls != 0 {
		t.Fatalf("Assess embedded the query %d time(s) — the hot path must stay embedding-free", emb.calls)
	}

	if _, err := ix.RetrieveFused(context.Background(), index.Query{Text: "quux"}, emb); err != nil {
		t.Fatalf("RetrieveFused: %v", err)
	}
	if emb.calls == 0 {
		t.Error("the pull path should embed the query")
	}
}

// EmbedCorpus is a no-op without an embedder; RetrieveFused then can't dense and
// returns the embedding-free result.
func TestEmbedCorpusNilIsNoOp(t *testing.T) {
	recs := []*record.Record{mkRecord(t, 1, "florb record", "the florb thing", nil, "Go", "")}
	ix := openIndex(t, recs)
	if err := ix.EmbedCorpus(context.Background(), recs, nil); err != nil {
		t.Fatalf("EmbedCorpus(nil): %v", err)
	}
	// embedder present at query time but no stored embeddings → dense empty.
	emb := &stubEmbedder{dim: 3, basis: map[string][]float32{"quux": {1, 0, 0}}}
	got, err := ix.RetrieveFused(context.Background(), index.Query{Text: "quux"}, emb)
	if err != nil {
		t.Fatalf("RetrieveFused: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("no stored embeddings → no dense hit; got %+v", got)
	}
}

func TestOllamaEmbedderParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float32{0.1, 0.2, 0.3}})
	}))
	defer srv.Close()

	emb := index.NewOllamaEmbedder(srv.URL, "")
	vec, err := emb.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 3 || vec[0] != 0.1 {
		t.Errorf("vec = %v, want [0.1 0.2 0.3]", vec)
	}
}

func TestOllamaEmbedderErrorsOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := index.NewOllamaEmbedder(srv.URL, "m").Embed(context.Background(), "x"); err == nil {
		t.Error("expected an error on HTTP 500")
	}
}

func sameHits(a, b []index.Hit) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Matched != b[i].Matched {
			return false
		}
	}
	return true
}
