// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// The orchestrator's gate for Assess: the ingest-time "is this present or new?"
// decision built on the existing retrieval pipeline. Known = an exact
// fingerprint hit (deterministic, present). Similar = a lexical near-match that
// cleared the floor — a detective's LEAD, returned WITH its evidence to verify,
// never an auto-merge verdict. Novel = nothing cleared the floor.

func similarCorpus(t *testing.T) []*record.Record {
	t.Helper()
	return []*record.Record{
		mkRecord(t, 10, "Postgres HNSW index build is slow under tiny maintenance_work_mem",
			"building an hnsw vector index takes hours when maintenance_work_mem is small",
			nil, "PyPI", "pgvector"),
		mkRecord(t, 11, "Cargo workspace feature unification breaks no_std builds",
			"a workspace member silently enables std features for everyone",
			nil, "crates.io", "cargo"),
	}
}

// An incoming error that matches a recorded signature modulo normalization is
// deterministically present: Known, with the fingerprint hit as evidence.
func TestAssessKnownOnFingerprintExact(t *testing.T) {
	ix := openIndex(t, corpus(t))
	a, err := ix.Assess(context.Background(), index.Query{Text: `FTS5: Syntax Error near "."`})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Novelty != index.NoveltyKnown {
		t.Fatalf("want Known, got %q (%+v)", a.Novelty, a)
	}
	if len(a.Candidates) == 0 {
		t.Fatal("Known must carry its evidence, got no candidates")
	}
	for _, c := range a.Candidates {
		if c.Matched != index.MatchedFingerprint {
			t.Errorf("Known evidence must be fingerprint hits, got %+v", c)
		}
	}
}

// No signature can match free text here (records carry none): a strong lexical
// overlap is a lead, not proof -> Similar, evidence is lexical, best-first.
func TestAssessSimilarOnLexicalNearMatch(t *testing.T) {
	ix := openIndex(t, similarCorpus(t))
	a, err := ix.Assess(context.Background(), index.Query{
		Text: "hnsw index build slow maintenance_work_mem vector",
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Novelty != index.NoveltySimilar {
		t.Fatalf("want Similar, got %q (%+v)", a.Novelty, a)
	}
	if len(a.Candidates) == 0 || a.Candidates[0].ID != "exp-0010" {
		t.Fatalf("want exp-0010 as the top lead, got %+v", a.Candidates)
	}
	// Exactly the single relevant lead: the unrelated cargo record (exp-0011)
	// must NOT be injected as a second lead by a floor/ranking regression. This
	// subsumes both the "exp-0011 absent" and the <=MaxK cap checks.
	if len(a.Candidates) != 1 {
		t.Fatalf("only exp-0010 should clear the floor; unrelated exp-0011 must be absent, got %+v", a.Candidates)
	}
	for _, c := range a.Candidates {
		if c.Matched != index.MatchedLexical {
			t.Errorf("Similar evidence must be lexical hits, got %+v", c)
		}
	}
}

// Nothing in the corpus overlaps: genuinely new work -> Novel, no evidence.
func TestAssessNovelWhenNothingMatches(t *testing.T) {
	ix := openIndex(t, similarCorpus(t))
	a, err := ix.Assess(context.Background(), index.Query{
		Text: "completely unrelated kangaroo helicopter xyzzy",
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Novelty != index.NoveltyNovel {
		t.Fatalf("want Novel, got %q (%+v)", a.Novelty, a)
	}
	if len(a.Candidates) != 0 {
		t.Errorf("Novel must carry no candidates, got %+v", a.Candidates)
	}
}

// ADR-0004: the relevance floor is index POLICY, not a per-call accident.
// Assess applies index.DefaultFloor unless the caller explicitly overrides it,
// so a weak single-token overlap that a floor-off search would call a lead is
// demoted to Novel BY DEFAULT — the locked ADR-0001 §3 invariant holds on every
// novelty path, not only when a caller remembered to pass Floor. The literal
// value of DefaultFloor is pinned here against the seed fixtures (ADR-0004 §4):
// it must sit above a single weak token yet below a genuine multi-term match.
func TestAssessAppliesDefaultFloorByPolicy(t *testing.T) {
	ix := openIndex(t, similarCorpus(t))

	// Premise — pin the boundary the constant must straddle. FloorOff yields the
	// raw lexical score, the only place the zero/off distinction is observable.
	weak, err := ix.Search(context.Background(), index.Query{Text: "index", Floor: index.FloorOff})
	if err != nil || len(weak) == 0 {
		t.Fatalf("setup weak search: %v %v", weak, err)
	}
	strong, err := ix.Search(context.Background(), index.Query{
		Text: "hnsw index build slow maintenance_work_mem vector", Floor: index.FloorOff,
	})
	if err != nil || len(strong) == 0 {
		t.Fatalf("setup strong search: %v %v", strong, err)
	}
	if !(weak[0].Score < index.DefaultFloor) {
		t.Fatalf("DefaultFloor (%g) must exceed a weak single token (%g)", index.DefaultFloor, weak[0].Score)
	}
	if !(strong[0].Score >= index.DefaultFloor) {
		t.Fatalf("DefaultFloor (%g) must not demote a multi-term match (%g)", index.DefaultFloor, strong[0].Score)
	}

	// Default policy (Floor unset): the weak single token falls below the floor → Novel.
	a, err := ix.Assess(context.Background(), index.Query{Text: "index"})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Novelty != index.NoveltyNovel {
		t.Fatalf("default floor: weak overlap must be Novel, got %q (%+v)", a.Novelty, a)
	}
	if len(a.Candidates) != 0 {
		t.Errorf("Novel must carry no candidates, got %+v", a.Candidates)
	}

	// The genuine multi-term match clears the default floor → Similar.
	a, err = ix.Assess(context.Background(), index.Query{
		Text: "hnsw index build slow maintenance_work_mem vector",
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Novelty != index.NoveltySimilar {
		t.Fatalf("default floor: multi-term match must survive as Similar, got %q", a.Novelty)
	}

	// Explicit opt-out (FloorOff) is the only way to get the weak lead back; the
	// zero value no longer means "off" at the Assess layer.
	a, err = ix.Assess(context.Background(), index.Query{Text: "index", Floor: index.FloorOff})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Novelty != index.NoveltySimilar {
		t.Fatalf("FloorOff override: weak overlap must return as Similar, got %q", a.Novelty)
	}
}

// ADR-0007: Retrieve is the injection-path search — the pull/push channels reach
// the corpus through it, so it applies the same DefaultFloor policy Assess does.
// A weak single-token overlap is floored to nothing ("empty is an answer"); only
// an explicit FloorOff brings raw recall back; a genuine multi-term match clears
// the floor. Search itself (ADR-0004) stays the floor-free mechanism.
func TestRetrieveAppliesFloorByPolicy(t *testing.T) {
	ix := openIndex(t, similarCorpus(t))

	// Default policy: the weak single token is floored out entirely.
	got, err := ix.Retrieve(context.Background(), index.Query{Text: "index"})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("default floor: weak overlap must return nothing, got %+v", got)
	}

	// Explicit opt-out yields the raw lead — the same hit Search returns floor-off.
	got, err = ix.Retrieve(context.Background(), index.Query{Text: "index", Floor: index.FloorOff})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("FloorOff override: weak overlap must return the raw hit")
	}

	// A genuine multi-term match clears the default floor.
	got, err = ix.Retrieve(context.Background(), index.Query{
		Text: "hnsw index build slow maintenance_work_mem vector",
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("default floor: multi-term match must survive")
	}
}

// Empty/whitespace query has nothing to match: Novel, never an error.
func TestAssessNovelOnEmptyText(t *testing.T) {
	ix := openIndex(t, similarCorpus(t))
	a, err := ix.Assess(context.Background(), index.Query{Text: "   "})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Novelty != index.NoveltyNovel || len(a.Candidates) != 0 {
		t.Errorf("empty text must be Novel with no candidates, got %+v", a)
	}
}

// Quarantine is honored exactly as Search honors it: a quarantined-only match
// is invisible to the push channel (Novel) but a lead to a pull-channel caller
// that opts in.
func TestAssessHonorsQuarantine(t *testing.T) {
	q := mkRecord(t, 30, "A quarantined lesson about flurbnix resets",
		"flurbnix counters reset after restart", nil, "Go", "example.com/flurbnix")
	q.Status = "quarantined"
	ix := openIndex(t, []*record.Record{q})

	a, err := ix.Assess(context.Background(), index.Query{Text: "flurbnix resets"})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Novelty != index.NoveltyNovel {
		t.Errorf("quarantined match must be invisible by default, got %q (%+v)", a.Novelty, a)
	}

	a, err = ix.Assess(context.Background(), index.Query{Text: "flurbnix resets", IncludeQuarantined: true})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Novelty != index.NoveltySimilar || len(a.Candidates) == 0 {
		t.Errorf("opt-in caller may see the quarantined lead, got %q (%+v)", a.Novelty, a)
	}
}

// Determinism: same input, same assessment.
func TestAssessDeterministic(t *testing.T) {
	ix := openIndex(t, similarCorpus(t))
	q := index.Query{Text: "hnsw index build slow maintenance_work_mem vector"}
	a, err := ix.Assess(context.Background(), q)
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	b, err := ix.Assess(context.Background(), q)
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	// Pin the reproducible-verdict property: same Novelty AND the same ordered
	// candidate identities (ID + Matched kind), not merely the same count — two
	// different candidate sets of equal length must NOT pass as deterministic.
	ids := func(hs []index.Hit) []string {
		out := make([]string, len(hs))
		for i, h := range hs {
			out[i] = h.ID + "|" + h.Matched
		}
		return out
	}
	if a.Novelty != b.Novelty || !reflect.DeepEqual(ids(a.Candidates), ids(b.Candidates)) {
		t.Errorf("non-deterministic: %+v vs %+v", a, b)
	}
}
