package index_test

import (
	"context"
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

// The relevance floor moves the Similar/Novel boundary: a weak single-term
// overlap that would be a lead with no floor must fall through to Novel once
// the floor is raised above it. Below the floor, returning nothing is a feature.
func TestAssessFloorDemotesWeakSimilarToNovel(t *testing.T) {
	ix := openIndex(t, similarCorpus(t))

	strongHits, err := ix.Search(context.Background(), index.Query{
		Text: "hnsw index build slow maintenance_work_mem vector",
	})
	if err != nil || len(strongHits) == 0 {
		t.Fatalf("setup search: %v %v", strongHits, err)
	}
	strong := strongHits[0].Score

	weakHits, err := ix.Search(context.Background(), index.Query{Text: "index"})
	if err != nil {
		t.Fatalf("setup search: %v", err)
	}
	if len(weakHits) == 0 || weakHits[0].Score >= strong {
		t.Fatalf("test premise broken: weak %v not below strong %v", weakHits, strong)
	}

	// No floor: the weak overlap is still a lead.
	a, err := ix.Assess(context.Background(), index.Query{Text: "index"})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Novelty != index.NoveltySimilar {
		t.Fatalf("no floor: want Similar, got %q", a.Novelty)
	}

	// Floor above the weak score: it drops to Novel.
	a, err = ix.Assess(context.Background(), index.Query{Text: "index", Floor: strong})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Novelty != index.NoveltyNovel {
		t.Fatalf("with floor: want Novel, got %q (%+v)", a.Novelty, a)
	}
	if len(a.Candidates) != 0 {
		t.Errorf("Novel must carry no candidates, got %+v", a.Candidates)
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
	if a.Novelty != b.Novelty || len(a.Candidates) != len(b.Candidates) {
		t.Errorf("non-deterministic: %+v vs %+v", a, b)
	}
}
