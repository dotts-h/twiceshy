// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/dotts-h/twiceshy/internal/fingerprint"
	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// mkQuarantinedTrap builds a minimal *quarantined* trap (no guard, no
// validated_at — neither is required until a record is validated).
func mkQuarantinedTrap(t *testing.T, num int, title, summary string, sigs []string) *record.Record {
	t.Helper()
	src := fmt.Sprintf(`---
schema_version: 1
id: exp-%04d
kind: trap
status: quarantined
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
	src += `applies_to:
  - ecosystem: Go
    package: example.com/x
resolution:
  root_cause: "a cause"
  fix: "a fix"
provenance:
  source: { author: "claude", session: null, pr: null }
  recorded_at: 2026-06-12
  valid: { from: 2026-06-12, until: null }
---

Quarantined narrative.
`
	rec, err := record.Parse(fmt.Sprintf("experience/2026/%04d-rec.md", num), []byte(src))
	if err != nil {
		t.Fatalf("quarantined fixture invalid: %v", err)
	}
	return rec
}

// H8: the race detector only catches a race it can observe. The rest of the
// suite issues no concurrent calls, so `go test -race` had nothing to inspect.
// Drive concurrent Search/Get/NextID against one shared *index.Index.
func TestConcurrentReadsAreRaceFree(t *testing.T) {
	ix := openIndex(t, corpus(t))
	ctx := context.Background()
	const workers = 16
	var wg sync.WaitGroup
	errCh := make(chan error, workers*3)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ix.Search(ctx, index.Query{Text: "fts5 syntax error near", K: 3, Floor: index.FloorOff}); err != nil {
				errCh <- err
			}
			if _, err := ix.Get(ctx, "exp-0001"); err != nil && !errors.Is(err, index.ErrNotFound) {
				errCh <- err
			}
			if _, err := ix.NextID(ctx); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent op failed: %v", err)
	}
}

// M9: Rebuild is a full wipe-and-reload, so a record dropped from the corpus
// must vanish from every table. Re-inserting the *same* set can't catch a
// missing DELETE; shrinking can.
func TestRebuildDropsRemovedRecords(t *testing.T) {
	ctx := context.Background()
	keep := mkRecord(t, 1, "kept trap about a zebra", "zebra summary kept", []string{"zebra-keep-sig"}, "Go", "a")
	drop := mkRecord(t, 2, "dropped trap about a quokka", "quokka summary dropped", []string{"quokka-drop-sig"}, "Go", "b")
	ix := openIndex(t, []*record.Record{keep, drop})

	if _, err := ix.Get(ctx, "exp-0002"); err != nil {
		t.Fatalf("exp-0002 should exist before shrink: %v", err)
	}
	if err := ix.Rebuild(ctx, []*record.Record{keep}, testRepo); err != nil {
		t.Fatalf("Rebuild (shrunk): %v", err)
	}
	if _, err := ix.Get(ctx, "exp-0002"); !errors.Is(err, index.ErrNotFound) {
		t.Errorf("dropped record must vanish from records, got err=%v", err)
	}
	// Its fingerprint row must be gone.
	fpHits, err := ix.Search(ctx, index.Query{Text: "quokka-drop-sig", Floor: index.FloorOff, IncludeQuarantined: true})
	if err != nil {
		t.Fatal(err)
	}
	if containsID(fpHits, "exp-0002") {
		t.Errorf("dropped record still fingerprint-searchable: %+v", fpHits)
	}
	// Its FTS row must be gone.
	lexHits, err := ix.Search(ctx, index.Query{Text: "quokka summary dropped", Floor: index.FloorOff, IncludeQuarantined: true})
	if err != nil {
		t.Fatal(err)
	}
	if containsID(lexHits, "exp-0002") {
		t.Errorf("dropped record still in FTS index: %+v", lexHits)
	}
}

// M6: the pull/push boundary (ADR-0001 §5/§6). Get returns a quarantined record
// by explicit id (the pull channel), but the default Search (push channel) must
// hide it; IncludeQuarantined is the deliberate opt-in.
func TestQuarantinePullGetVsPushSearch(t *testing.T) {
	ctx := context.Background()
	q := mkQuarantinedTrap(t, 50, "quarantined draft about a walrus", "walrus draft summary", []string{"walrus-draft-sig"})
	ix := openIndex(t, []*record.Record{q})

	got, err := ix.Get(ctx, "exp-0050")
	if err != nil {
		t.Fatalf("Get must return the quarantined record by id: %v", err)
	}
	if got.Status != "quarantined" {
		t.Errorf("status = %q, want quarantined", got.Status)
	}
	hits, err := ix.Search(ctx, index.Query{Text: "walrus-draft-sig", Floor: index.FloorOff})
	if err != nil {
		t.Fatal(err)
	}
	if containsID(hits, "exp-0050") {
		t.Errorf("quarantined record leaked into default (push) search: %+v", hits)
	}
	inc, err := ix.Search(ctx, index.Query{Text: "walrus-draft-sig", Floor: index.FloorOff, IncludeQuarantined: true})
	if err != nil {
		t.Fatal(err)
	}
	if !containsID(inc, "exp-0050") {
		t.Errorf("IncludeQuarantined must surface the quarantined record, got %+v", inc)
	}
}

// M8: the fingerprint stored at index time must agree with the algorithm run at
// query time — including normalization. A record indexed from one signature is
// hit by a normalization-equivalent query (digits → <num>), proving the stored
// fp is the normalized algorithm output, not the raw string.
func TestStoredFingerprintAgreesWithAlgorithm(t *testing.T) {
	ctx := context.Background()
	sigA := "connection timed out after 30 seconds on node 5"
	sigB := "connection timed out after 999 seconds on node 8"
	if fingerprint.Generic(sigA) != fingerprint.Generic(sigB) {
		t.Fatal("test premise broken: the two signatures should normalize to the same fingerprint")
	}
	rec := mkRecord(t, 7, "timeout trap on a node", "timeout summary", []string{sigA}, "Go", "a")
	ix := openIndex(t, []*record.Record{rec})

	exact, err := ix.Search(ctx, index.Query{Text: sigA, Floor: index.FloorOff})
	if err != nil {
		t.Fatal(err)
	}
	if !matchedFingerprint(exact, "exp-0007") {
		t.Fatalf("exact signature must hit via fingerprint, got %+v", exact)
	}
	norm, err := ix.Search(ctx, index.Query{Text: sigB, Floor: index.FloorOff})
	if err != nil {
		t.Fatal(err)
	}
	if !matchedFingerprint(norm, "exp-0007") {
		t.Errorf("normalization-equivalent signature must hit the same fingerprint — stored fp drifted from the algorithm; got %+v", norm)
	}
}

func containsID(hits []index.Hit, id string) bool {
	for _, h := range hits {
		if h.ID == id {
			return true
		}
	}
	return false
}

func matchedFingerprint(hits []index.Hit, id string) bool {
	for _, h := range hits {
		if h.ID == id && h.Matched == index.MatchedFingerprint {
			return true
		}
	}
	return false
}
