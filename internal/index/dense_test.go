// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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

// zeroVecEmbedder returns an EMPTY vector with NO error — a degenerate but
// real failure mode that RetrieveFused must treat as "can't embed" and fall
// back to the embedding-free path (dense.go: len(qvec)==0).
type zeroVecEmbedder struct{ calls int }

func (z *zeroVecEmbedder) Embed(context.Context, string) ([]float32, error) {
	z.calls++
	return []float32{}, nil
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
	// The fused Hit.Score is the RRF fused score (caller-visible, ranked on by
	// push/pull consumers), NOT the raw cosine. A single dense-only hit ranked
	// first is exactly 1/(rrfK+1) = 1/61; the raw cosine here is 1.0, distinct
	// from 1/61, so this also catches a raw-cosine leak.
	const wantRRF = 1.0 / 61.0 // single dense-only hit at rank 1: 1/(rrfK+1), rrfK=60 (dense.go)
	if got := fused[0].Score; got <= 0 {
		t.Errorf("fused dense hit must carry a positive RRF score, got %v", got)
	} else if diff := got - wantRRF; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("fused Score = %v, want RRF %v (not the raw cosine 1.0)", got, wantRRF)
	}
}

// RRF fusion must NOT smuggle in a record that neither matches lexically NOR
// clears the dense floor. The far record below shares no token with the query
// (a non-match, not a floored lexical match) AND embeds orthogonally (cosine 0 <
// denseFloor), so it must be absent from the fused result. The BM25-floor-under-
// fusion case is covered separately by TestRetrieveFusedBM25FloorGatesUnderFusion.
func TestRetrieveFusedDropsOrthogonalNonMatch(t *testing.T) {
	emb := &stubEmbedder{dim: 3, basis: map[string][]float32{
		"quux":  {1, 0, 0}, // query
		"florb": {1, 0, 0}, // near record: same basis → cosine 1 (clears dense floor)
		"zonk":  {0, 1, 0}, // far record: orthogonal → cosine 0 (below dense floor)
	}}
	recs := []*record.Record{
		mkRecord(t, 1, "Near via embedding", "the florb subsystem stalls", nil, "Go", ""),
		mkRecord(t, 2, "Doubly below floor", "the zonk widget is unrelated", nil, "Go", ""),
	}
	ix := openIndex(t, recs)
	embedAll(t, ix, recs, emb)

	// "quux" shares no lexical token with either record (both miss BM25); it
	// embeds near exp-0001 (cosine 1) and orthogonal to exp-0002 (cosine 0).
	fused, err := ix.RetrieveFused(context.Background(), index.Query{Text: "quux"}, emb)
	if err != nil {
		t.Fatalf("RetrieveFused: %v", err)
	}
	for _, h := range fused {
		if h.ID == "exp-0002" {
			t.Fatalf("doubly-below-floor record exp-0002 must not be fused in; got %+v", fused)
		}
	}
	if len(fused) != 1 || fused[0].ID != "exp-0001" {
		t.Fatalf("only the above-floor record should appear; got %+v", fused)
	}
}

// A record below the BM25 lexical floor cannot be smuggled into the fused
// result via RRF — the floor clause (index.go lexicalHits) is provably the gate.
// The weak record DOES enter the lexical channel (it shares a token, proven with
// FloorOff), but a Floor set above its bm25 score (and below the control's)
// excludes it under fusion; dense contributes nothing here (no matching
// embedding), so only the lexical floor can do the excluding.
func TestRetrieveFusedBM25FloorGatesUnderFusion(t *testing.T) {
	// No basis matches any record text → every embedding is the zero vector,
	// cosine 0 < denseFloor → dense returns nothing. Lexical is the only channel.
	emb := &stubEmbedder{dim: 3, basis: map[string][]float32{"nonexistent": {1, 0, 0}}}
	recs := []*record.Record{
		mkRecord(t, 1, "Strong control gualzborg widget lesson",
			"the gualzborg subsystem mishandles widget traffic", nil, "Go", ""),
		mkRecord(t, 2, "Weak lonely widget lesson",
			"a lonely widget appears here", nil, "Go", ""),
	}
	ix := openIndex(t, recs)
	embedAll(t, ix, recs, emb)

	q := index.Query{Text: "gualzborg widget"}

	// Measure the raw lexical scores floor-off: the control outranks the weak
	// single-token record, so a floor between them is achievable.
	raw, err := ix.Search(context.Background(), index.Query{Text: q.Text, Floor: index.FloorOff})
	if err != nil {
		t.Fatalf("floor-off Search: %v", err)
	}
	var control, weak float64
	for _, h := range raw {
		switch h.ID {
		case "exp-0001":
			control = h.Score
		case "exp-0002":
			weak = h.Score
		}
	}
	if control == 0 || weak == 0 || !(weak < control) {
		t.Fatalf("test premise: need weak (%v) below control (%v), both >0", weak, control)
	}

	// (1) FloorOff: the weak record IS fused in — it genuinely enters the lexical
	// channel and would otherwise survive.
	fused, err := ix.RetrieveFused(context.Background(), index.Query{Text: q.Text, Floor: index.FloorOff}, emb)
	if err != nil {
		t.Fatalf("RetrieveFused FloorOff: %v", err)
	}
	if !containsID(fused, "exp-0002") {
		t.Fatalf("floor-off: weak record must enter the lexical channel; got %+v", fused)
	}

	// (2) Floor set between weak and control: the BM25 floor — not the dense floor,
	// not tokenization — excludes the weak record while keeping the control.
	floor := (weak + control) / 2
	fused, err = ix.RetrieveFused(context.Background(), index.Query{Text: q.Text, Floor: floor}, emb)
	if err != nil {
		t.Fatalf("RetrieveFused floored: %v", err)
	}
	if !containsID(fused, "exp-0001") {
		t.Fatalf("floored: the control must survive its own floor; got %+v", fused)
	}
	if containsID(fused, "exp-0002") {
		t.Fatalf("floored: the below-BM25-floor record must not be smuggled in via RRF; got %+v", fused)
	}
}

// denseFloor (dense.go) is a guarding-test-owned constant: it must sit below
// 0.6 yet above 0.4472 so a record at cosine 0.6 is fused in and one at 0.4472
// is dropped. Bracket it tightly with a basis that shares no lexical token with
// the query (so BM25 contributes nothing and the dense floor is isolated):
// query→{1,0}; record-A→{3,4} (cosine 0.6, just above the 0.5 floor); record-B
// →{1,2} (cosine 0.4472, just below). Mutation check: lowering denseFloor to
// 0.4 pulls B in; raising it to 0.65 drops A out.
func TestRetrieveFusedPinsDenseFloorBoundary(t *testing.T) {
	emb := &stubEmbedder{dim: 2, basis: map[string][]float32{
		"qqxx": {1, 0}, // query
		"wwzz": {3, 4}, // record-A: cosine with {1,0} = 3/5 = 0.6  (above floor)
		"vvkk": {1, 2}, // record-B: cosine with {1,0} = 1/√5 = 0.4472 (below floor)
	}}
	recs := []*record.Record{
		mkRecord(t, 1, "Just above the dense floor", "the wwzz subsystem misbehaves", nil, "Go", ""),
		mkRecord(t, 2, "Just below the dense floor", "the vvkk subsystem misbehaves", nil, "Go", ""),
	}
	ix := openIndex(t, recs)
	embedAll(t, ix, recs, emb)

	q := index.Query{Text: "qqxx"} // shares no lexical token with either record
	plain, err := ix.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(plain) != 0 {
		t.Fatalf("query must miss lexically so only the dense floor gates; got %d hits", len(plain))
	}
	fused, err := ix.RetrieveFused(context.Background(), q, emb)
	if err != nil {
		t.Fatalf("RetrieveFused: %v", err)
	}
	if len(fused) != 1 || fused[0].ID != "exp-0001" {
		t.Fatalf("only the cosine-0.6 record (exp-0001) clears denseFloor; got %+v", fused)
	}
	for _, h := range fused {
		if h.ID == "exp-0002" {
			t.Errorf("cosine-0.4472 record (exp-0002) is below denseFloor and must be dropped; got %+v", fused)
		}
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

// denseHits sorts candidates by DESCENDING cosine (id tie-break) and truncates
// to k. Drive both with DISTINCT, KNOWN cosines so the comparator and the k-cap
// drop are actually exercised: query→{1,0,0}; five records at cosines 1.0, 0.96,
// 0.8, 0.6 (all above the 0.5 floor) and 0 (below). With FOUR above-floor and
// MaxK=3 the cap MUST drop the least-similar above-floor record. No record
// shares a lexical token with the query, so lexicalHits returns nil and the
// fused order mirrors dense rank order.
func TestRetrieveFusedDenseOrdersByDescendingSimilarity(t *testing.T) {
	emb := &stubEmbedder{dim: 3, basis: map[string][]float32{
		"qqxx": {1, 0, 0},       // query
		"aaaa": {1, 0, 0},       // cosine 1.0
		"bbbb": {0.96, 0.28, 0}, // cosine 0.96
		"cccc": {0.8, 0.6, 0},   // cosine 0.8
		"dddd": {0.6, 0.8, 0},   // cosine 0.6 (above floor, but least similar)
		"eeee": {0, 1, 0},       // cosine 0 (below floor)
	}}
	recs := []*record.Record{
		mkRecord(t, 1, "Closest aaaa record", "the aaaa subsystem", nil, "Go", ""),
		mkRecord(t, 2, "Second bbbb record", "the bbbb subsystem", nil, "Go", ""),
		mkRecord(t, 3, "Third cccc record", "the cccc subsystem", nil, "Go", ""),
		mkRecord(t, 4, "Fourth dddd record", "the dddd subsystem", nil, "Go", ""),
		mkRecord(t, 5, "Below floor eeee record", "the eeee subsystem", nil, "Go", ""),
	}
	ix := openIndex(t, recs)
	embedAll(t, ix, recs, emb)

	q := index.Query{Text: "qqxx"}
	plain, err := ix.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(plain) != 0 {
		t.Fatalf("query must miss lexically so only dense ranks; got %d hits", len(plain))
	}
	fused, err := ix.RetrieveFused(context.Background(), q, emb)
	if err != nil {
		t.Fatalf("RetrieveFused: %v", err)
	}

	// (1) ORDER: strictly descending cosine → exp-0001, exp-0002, exp-0003.
	gotIDs := make([]string, len(fused))
	for i, h := range fused {
		gotIDs[i] = h.ID
	}
	want := []string{"exp-0001", "exp-0002", "exp-0003"}
	if !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("dense order = %v, want %v (descending cosine)", gotIDs, want)
	}
	// (2) K-CAP DROP: the least-similar above-floor record (exp-0004, cosine 0.6)
	// is dropped because the cap fits only three of four above-floor candidates.
	// (3) FLOOR: exp-0005 (cosine 0) is below denseFloor.
	for _, h := range fused {
		if h.ID == "exp-0004" {
			t.Errorf("least-similar above-floor record exp-0004 must be cap-dropped, not kept; got %+v", fused)
		}
		if h.ID == "exp-0005" {
			t.Errorf("below-floor record exp-0005 must be absent; got %+v", fused)
		}
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

// An embedder that yields an EMPTY vector WITHOUT erroring degrades to the
// embedding-free path, exactly as the error case does (dense.go len(qvec)==0).
func TestRetrieveFusedEmptyVectorFallsBack(t *testing.T) {
	recs := []*record.Record{
		mkRecord(t, 1, "FTS5 MATCH trap", "raw user input breaks the query", []string{"fts5: syntax error"}, "MCP", ""),
	}
	ix := openIndex(t, recs)
	q := index.Query{Text: "fts5 syntax error"}
	want, _ := ix.Retrieve(context.Background(), q)
	zero := &zeroVecEmbedder{}
	got, err := ix.RetrieveFused(context.Background(), q, zero)
	if err != nil {
		t.Fatalf("RetrieveFused must not error on an empty embedding: %v", err)
	}
	if zero.calls == 0 {
		t.Error("expected the embedder to be attempted")
	}
	if !sameHits(want, got) {
		t.Errorf("empty-vector fallback != Retrieve\n want %+v\n got %+v", want, got)
	}
}

// OllamaEmbedder rejects a successful (200) response whose embedding array is
// empty — the empty-embedding guard (dense.go).
func TestOllamaEmbedderErrorsOnEmptyEmbedding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float32{}})
	}))
	defer srv.Close()
	_, err := index.NewOllamaEmbedder(srv.URL, "m").Embed(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "empty embedding") {
		t.Errorf("want an 'empty embedding' error, got %v", err)
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

// EmbedCorpus caches embeddings by content hash in embedding_cache, so a
// re-embed (e.g. on every server start) only embeds records whose text changed.
// Pins that CONTRACT via the stubEmbedder.calls counter: a second pass over the
// SAME content does zero embeds (all cache hits survive the DELETE FROM
// embeddings), and changing one record's text triggers exactly one re-embed.
func TestEmbedCorpusCachesByContentHash(t *testing.T) {
	emb := &stubEmbedder{dim: 3, basis: map[string][]float32{"florb": {1, 0, 0}}}
	recs := []*record.Record{
		mkRecord(t, 1, "Florb subsystem trap", "the florb thing stalls", nil, "Go", ""),
		mkRecord(t, 2, "Another unrelated lesson", "totally different words", nil, "Go", ""),
	}
	ix := openIndex(t, recs)

	// First pass: every record is a genuine cache miss → one embed each.
	embedAll(t, ix, recs, emb)
	if emb.calls != len(recs) {
		t.Fatalf("first pass should embed each record once; calls=%d, want %d", emb.calls, len(recs))
	}

	// Second pass over the SAME content: all cache hits, zero re-embeds.
	emb.calls = 0
	embedAll(t, ix, recs, emb)
	if emb.calls != 0 {
		t.Fatalf("unchanged content must hit the cache; re-embedded %d time(s)", emb.calls)
	}

	// Change one record's text (new embedText → new hash → one cache miss).
	recs[0].Title = "Florb subsystem trap (revised)"
	emb.calls = 0
	embedAll(t, ix, recs, emb)
	if emb.calls != 1 {
		t.Fatalf("only the changed record should re-embed; got %d embeds, want 1", emb.calls)
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
