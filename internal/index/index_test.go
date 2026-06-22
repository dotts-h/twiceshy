// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/testcorpus"
)

const testRepo = "github.com/dotts-h/twiceshy"

// mkRecord assembles a minimal validated trap record in memory.
func mkRecord(t *testing.T, num int, title, summary string, sigs []string, eco, pkg string) *record.Record {
	t.Helper()
	gt := "TestSomething"
	src := fmt.Sprintf(`---
schema_version: 1
id: exp-%04d
kind: trap
status: validated
title: %q
symptom:
  summary: %q
`, num, title, summary)
	if len(sigs) > 0 {
		src += "  error_signatures:\n"
		for _, s := range sigs {
			src += fmt.Sprintf("    - %q\n", s)
		}
	}
	src += fmt.Sprintf(`applies_to:
  - ecosystem: %q
    package: %q
resolution:
  root_cause: "a cause"
  fix: "a fix"
guard: { repro: null, guarding_test: %q }
provenance:
  source: { author: "horia", session: null, pr: null }
  recorded_at: 2026-06-12
  validated_at: 2026-06-12
  valid: { from: 2026-06-12, until: null }
  superseded_by: null
---

Narrative for %s.
`, eco, pkg, gt, title)
	rec, err := record.Parse(fmt.Sprintf("experience/2026/%04d-rec.md", num), []byte(src))
	if err != nil {
		t.Fatalf("fixture record invalid: %v", err)
	}
	return rec
}

func openIndex(t *testing.T, recs []*record.Record) *index.Index {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, testRepo); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	return ix
}

func corpus(t *testing.T) []*record.Record {
	t.Helper()
	recs, err := record.LoadCorpus(testcorpus.Root())
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	return recs
}

func TestGetReturnsFullRecord(t *testing.T) {
	ix := openIndex(t, corpus(t))
	got, err := ix.Get(context.Background(), "exp-0001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Kind != "trap" || got.Title == "" || got.Path == "" {
		t.Errorf("meta incomplete: %+v", got)
	}
	for _, want := range []string{"schema_version: 1", "## The trap"} {
		if !strings.Contains(got.Markdown, want) {
			t.Errorf("Markdown should contain %q", want)
		}
	}
}

func TestGetUnknownIDIsErrNotFound(t *testing.T) {
	ix := openIndex(t, corpus(t))
	_, err := ix.Get(context.Background(), "exp-9999")
	if !errors.Is(err, index.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// Fingerprint-exact retrieval: an incoming error message that matches a
// recorded signature modulo normalization must hit first, marked as a
// fingerprint match (ADR-0001 §3 precedence).
func TestSearchFingerprintExactWinsOverLexical(t *testing.T) {
	ix := openIndex(t, corpus(t))
	hits, err := ix.Search(context.Background(), index.Query{
		Text: `FTS5: Syntax Error near "."`,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	if hits[0].ID != "exp-0001" || hits[0].Matched != index.MatchedFingerprint {
		t.Errorf("hits[0] = %+v, want exp-0001 via fingerprint", hits[0])
	}
}

// Guarding test for exp-0001: raw user input must be escaped before it
// reaches FTS5 MATCH — punctuation-heavy queries are valid queries here.
func TestSearchQuoteEscapesFTS5Input(t *testing.T) {
	ix := openIndex(t, corpus(t))
	hostile := []string{
		`modernc.org/sqlite`,
		`utf-8 node.js`,
		`"unbalanced quote`,
		`AND OR NOT NEAR(`,
		`-col:^prefix*`,
		`. - / " ( ) *`,
		`'); DROP TABLE records; --`,
	}
	for _, q := range hostile {
		if _, err := ix.Search(context.Background(), index.Query{Text: q}); err != nil {
			t.Errorf("Search(%q) errored: %v", q, err)
		}
	}
	// And escaping must not break retrieval itself.
	hits, err := ix.Search(context.Background(), index.Query{Text: "bm25 DESC sorts garbage FTS5"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 || hits[0].ID != "exp-0002" {
		t.Errorf("want exp-0002 first for a bm25 query, got %+v", hits)
	}
}

// Guarding test for exp-0002: SQLite bm25() is negative, lower-is-better.
// Ordering and the relevance floor must follow that convention; exported
// scores are positive (sign flipped exactly once at the boundary).
func TestSearchRelevanceFloorUsesBM25Convention(t *testing.T) {
	recs := []*record.Record{
		mkRecord(t, 10, "Postgres HNSW index build is slow under tiny maintenance_work_mem",
			"building an hnsw vector index takes hours when maintenance_work_mem is small",
			nil, "PyPI", "pgvector"),
		mkRecord(t, 11, "Cargo workspace feature unification breaks no_std builds",
			"a workspace member silently enables std features for everyone",
			nil, "crates.io", "cargo"),
	}
	ix := openIndex(t, recs)

	// Strongly relevant query: many overlapping terms with record 10 only.
	hits, err := ix.Search(context.Background(), index.Query{
		Text: "hnsw index build slow maintenance_work_mem vector",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 || hits[0].ID != "exp-0010" {
		t.Fatalf("want exp-0010 first, got %+v", hits)
	}
	for _, h := range hits {
		if h.Score <= 0 {
			t.Errorf("exported scores must be positive (flip the bm25 sign once): %+v", h)
		}
	}
	if len(hits) > 1 && hits[0].Score < hits[1].Score {
		t.Error("hits must be ordered best-first by positive score")
	}

	// A weak single-token overlap ("index") must drown below a floor that a
	// strong multi-term match clears: with the floor raised to sit between
	// the two, only the strong match survives. Below the floor, NOTHING.
	strong := hits[0].Score
	weakHits, err := ix.Search(context.Background(), index.Query{Text: "index"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(weakHits) > 0 && weakHits[0].Score >= strong {
		t.Fatalf("test premise broken: weak query scored %v >= strong %v", weakHits[0].Score, strong)
	}
	floor := strong // floor at exactly the strong score keeps it (>=), kills weaker
	hits, err = ix.Search(context.Background(), index.Query{
		Text:  "hnsw index build slow maintenance_work_mem vector",
		Floor: floor,
	})
	if err != nil || len(hits) == 0 {
		t.Fatalf("strong match must survive its own score as floor: %v %v", hits, err)
	}
	weakHits, err = ix.Search(context.Background(), index.Query{Text: "index", Floor: floor})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(weakHits) != 0 {
		t.Errorf("below the relevance floor nothing may be returned, got %+v", weakHits)
	}
}

func TestSearchCapsKAtThree(t *testing.T) {
	var recs []*record.Record
	for i := 20; i < 27; i++ {
		recs = append(recs, mkRecord(t, i,
			fmt.Sprintf("Yet another zorblefrag failure mode number %d", i),
			"the zorblefrag subsystem exploded again",
			nil, "Go", "example.com/zorblefrag"))
	}
	ix := openIndex(t, recs)
	hits, err := ix.Search(context.Background(), index.Query{Text: "zorblefrag exploded", K: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) > 3 {
		t.Errorf("hard cap is k<=3 (ADR-0001 §3), got %d hits", len(hits))
	}
}

func TestSearchExcludesQuarantinedByDefault(t *testing.T) {
	q := mkRecord(t, 30, "A quarantined lesson about flurbnix resets",
		"flurbnix counters reset after restart", nil, "Go", "example.com/flurbnix")
	q.Status = "quarantined"
	ix := openIndex(t, []*record.Record{q})

	hits, err := ix.Search(context.Background(), index.Query{Text: "flurbnix resets"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("quarantined records must not surface by default, got %+v", hits)
	}

	hits, err = ix.Search(context.Background(), index.Query{Text: "flurbnix resets", IncludeQuarantined: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].Status != "quarantined" {
		t.Errorf("pull channel may surface quarantined records labeled as such, got %+v", hits)
	}
}

func TestSearchFiltersByStackFingerprint(t *testing.T) {
	ix := openIndex(t, corpus(t))
	hits, err := ix.Search(context.Background(), index.Query{
		Text:      "deprecated transport sqlite fts5 syntax",
		Ecosystem: "MCP",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, h := range hits {
		if h.ID != "exp-0003" {
			t.Errorf("ecosystem filter leaked %s", h.ID)
		}
	}
}

func TestRebuildIsIdempotent(t *testing.T) {
	recs := corpus(t)
	ix := openIndex(t, recs)
	if err := ix.Rebuild(context.Background(), recs, testRepo); err != nil {
		t.Fatalf("second Rebuild: %v", err)
	}
	hits, err := ix.Search(context.Background(), index.Query{Text: "fts5 bm25 negative"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	seen := map[string]bool{}
	for _, h := range hits {
		if seen[h.ID] {
			t.Errorf("duplicate hit %s after rebuild", h.ID)
		}
		seen[h.ID] = true
	}
}
