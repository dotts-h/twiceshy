// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

func usageIndex(t *testing.T, recs ...*record.Record) *index.Index {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, "github.com/dotts-h/twiceshy"); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	return ix
}

func usageRec(id string) *record.Record {
	return &record.Record{
		SchemaVersion: 1, ID: id, Kind: "trap", Status: "validated",
		Title: "a record for usage counting tests, long enough title",
		Path:  "experience/2026/" + id[4:] + "-x.md",
		Provenance: record.Provenance{
			Source:     record.Source{Author: "test"},
			RecordedAt: "2026-06-19",
			Valid:      record.Validity{From: "2026-06-19"},
		},
	}
}

func TestRecordHitsIncrementsAndSetsLastHit(t *testing.T) {
	ix := usageIndex(t, usageRec("exp-0100"))
	ctx := context.Background()

	if err := ix.RecordHits(ctx, []string{"exp-0100"}, "2026-06-19"); err != nil {
		t.Fatalf("RecordHits: %v", err)
	}
	u, err := ix.Usage(ctx, "exp-0100")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if u.Retrieved != 1 {
		t.Fatalf("retrieved = %d, want 1", u.Retrieved)
	}
	if u.LastHit == nil || *u.LastHit != "2026-06-19" {
		t.Fatalf("last_hit = %v, want 2026-06-19", u.LastHit)
	}

	// A second hit advances monotonically and updates last_hit.
	if err := ix.RecordHits(ctx, []string{"exp-0100"}, "2026-06-20"); err != nil {
		t.Fatal(err)
	}
	u, _ = ix.Usage(ctx, "exp-0100")
	if u.Retrieved != 2 || *u.LastHit != "2026-06-20" {
		t.Fatalf("after 2nd hit: retrieved=%d last_hit=%v, want 2 / 2026-06-20", u.Retrieved, *u.LastHit)
	}
}

func TestRecordHitsEmptyAndUnseen(t *testing.T) {
	ix := usageIndex(t, usageRec("exp-0100"))
	ctx := context.Background()

	if err := ix.RecordHits(ctx, nil, "2026-06-19"); err != nil {
		t.Fatalf("empty RecordHits must be a no-op, got %v", err)
	}
	// Usage of a record never hit is the zero value, not an error.
	u, err := ix.Usage(ctx, "exp-0100")
	if err != nil {
		t.Fatalf("Usage(unseen): %v", err)
	}
	if u.Retrieved != 0 || u.ConfirmedHelpful != 0 || u.LastHit != nil {
		t.Fatalf("unseen usage = %+v, want zero", u)
	}
}

func TestConfirmHelpful(t *testing.T) {
	ix := usageIndex(t, usageRec("exp-0100"))
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := ix.ConfirmHelpful(ctx, "exp-0100"); err != nil {
			t.Fatalf("ConfirmHelpful: %v", err)
		}
	}
	u, _ := ix.Usage(ctx, "exp-0100")
	if u.ConfirmedHelpful != 2 {
		t.Fatalf("confirmed_helpful = %d, want 2", u.ConfirmedHelpful)
	}
	// confirmed_helpful is independent of retrieved.
	if u.Retrieved != 0 {
		t.Fatalf("retrieved leaked to %d, want 0", u.Retrieved)
	}
}

func TestUsageSurvivesRebuild(t *testing.T) {
	ix := usageIndex(t, usageRec("exp-0100"))
	ctx := context.Background()
	if err := ix.RecordHits(ctx, []string{"exp-0100"}, "2026-06-19"); err != nil {
		t.Fatal(err)
	}
	// Rebuild (e.g. a server restart re-indexing the corpus) must not wipe the
	// accumulated usage signal — it is the one non-derived state in the index.
	if err := ix.Rebuild(ctx, []*record.Record{usageRec("exp-0100")}, "github.com/dotts-h/twiceshy"); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	u, _ := ix.Usage(ctx, "exp-0100")
	if u.Retrieved != 1 {
		t.Fatalf("after Rebuild retrieved = %d, want 1 (usage must survive rebuild)", u.Retrieved)
	}
}

func TestRecordHitsConcurrentMonotonic(t *testing.T) {
	ix := usageIndex(t, usageRec("exp-0100"))
	ctx := context.Background()
	const goroutines, perG = 8, 25

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				if err := ix.RecordHits(ctx, []string{"exp-0100"}, "2026-06-19"); err != nil {
					t.Errorf("RecordHits: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	u, _ := ix.Usage(ctx, "exp-0100")
	if want := goroutines * perG; u.Retrieved != want {
		t.Fatalf("retrieved = %d, want %d — concurrent increments lost an update", u.Retrieved, want)
	}
}
