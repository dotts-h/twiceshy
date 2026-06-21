// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite" // driver for the legacy-schema migration test

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

func TestRecordPushesIncrementsPushedOnly(t *testing.T) {
	ix := usageIndex(t, usageRec("exp-0100"))
	ctx := context.Background()

	if err := ix.RecordPushes(ctx, []string{"exp-0100"}); err != nil {
		t.Fatalf("RecordPushes: %v", err)
	}
	if err := ix.RecordPushes(ctx, []string{"exp-0100"}); err != nil {
		t.Fatalf("RecordPushes 2: %v", err)
	}
	u, err := ix.Usage(ctx, "exp-0100")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if u.Pushed != 2 {
		t.Fatalf("pushed = %d, want 2 (monotonic)", u.Pushed)
	}
	// A push impression is a distinct, weaker signal: it must NOT touch the
	// deliberate-pull counter or last_hit (staleness stays tied to real retrieval).
	if u.Retrieved != 0 {
		t.Fatalf("retrieved = %d, want 0 (a push must not bump retrieved)", u.Retrieved)
	}
	if u.LastHit != nil {
		t.Fatalf("last_hit = %v, want nil (a push must not set last_hit)", u.LastHit)
	}

	all, err := ix.AllUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if all["exp-0100"].Pushed != 2 {
		t.Fatalf("AllUsage pushed = %d, want 2", all["exp-0100"].Pushed)
	}

	if err := ix.RecordPushes(ctx, nil); err != nil {
		t.Fatalf("RecordPushes(nil) must be a no-op: %v", err)
	}
}

// TestOpenMigratesLegacyUsageSchema guards the additive migration: an index file
// created before the `pushed` column (the live /data volume) must gain it in
// place on Open, not fail — the index is derived, but the usage table survives
// Rebuild so a live db cannot just be dropped.
func TestOpenMigratesLegacyUsageSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE usage (
		record_id TEXT PRIMARY KEY,
		retrieved INTEGER NOT NULL DEFAULT 0,
		confirmed_helpful INTEGER NOT NULL DEFAULT 0,
		last_hit TEXT)`); err != nil {
		t.Fatalf("create legacy usage table: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO usage (record_id, retrieved) VALUES ('exp-0100', 5)`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	ix, err := index.Open(dbPath) // must ADD COLUMN pushed, preserving existing rows
	if err != nil {
		t.Fatalf("Open legacy db: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	ctx := context.Background()
	if err := ix.RecordPushes(ctx, []string{"exp-0100"}); err != nil {
		t.Fatalf("RecordPushes after migration: %v", err)
	}
	u, err := ix.Usage(ctx, "exp-0100")
	if err != nil {
		t.Fatal(err)
	}
	if u.Pushed != 1 {
		t.Fatalf("pushed = %d, want 1 after migration", u.Pushed)
	}
	if u.Retrieved != 5 {
		t.Fatalf("retrieved = %d, want 5 (migration must preserve existing counters)", u.Retrieved)
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

func TestAllUsage(t *testing.T) {
	ix := usageIndex(t, usageRec("exp-0100"), usageRec("exp-0101"))
	ctx := context.Background()

	if err := ix.RecordHits(ctx, []string{"exp-0100"}, "2026-06-19"); err != nil {
		t.Fatalf("RecordHits: %v", err)
	}
	if err := ix.RecordHits(ctx, []string{"exp-0100"}, "2026-06-19"); err != nil {
		t.Fatalf("RecordHits 2nd: %v", err)
	}
	if err := ix.ConfirmHelpful(ctx, "exp-0100"); err != nil {
		t.Fatalf("ConfirmHelpful: %v", err)
	}

	got, err := ix.AllUsage(ctx)
	if err != nil {
		t.Fatalf("AllUsage: %v", err)
	}
	want := map[string]record.Usage{
		"exp-0100": {Retrieved: 2, ConfirmedHelpful: 1, LastHit: strPtr("2026-06-19")},
	}
	if len(got) != len(want) {
		t.Fatalf("AllUsage len = %d, want %d; got %+v", len(got), len(want), got)
	}
	for id, w := range want {
		u, ok := got[id]
		if !ok {
			t.Fatalf("AllUsage missing %s", id)
		}
		if u.Retrieved != w.Retrieved || u.ConfirmedHelpful != w.ConfirmedHelpful {
			t.Fatalf("%s: %+v, want %+v", id, u, w)
		}
		if u.LastHit == nil || *u.LastHit != *w.LastHit {
			t.Fatalf("%s: last_hit = %v, want %v", id, u.LastHit, w.LastHit)
		}
	}
	if _, ok := got["exp-0101"]; ok {
		t.Fatalf("exp-0101 should be absent (no usage row), got %+v", got["exp-0101"])
	}
}

func TestAllUsageEmptyTable(t *testing.T) {
	ix := usageIndex(t, usageRec("exp-0100"))
	got, err := ix.AllUsage(context.Background())
	if err != nil {
		t.Fatalf("AllUsage: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty usage table: got %+v, want empty map", got)
	}
}

func strPtr(s string) *string { return &s }

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
