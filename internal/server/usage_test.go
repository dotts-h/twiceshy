// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

const usageTestRepo = "github.com/dotts-h/twiceshy"

func fixedClock() time.Time { return time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC) }

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func usageFixture() *record.Record {
	return &record.Record{
		SchemaVersion: 1, ID: "exp-0200", Kind: "trap", Status: "validated",
		Title: "usage wiring fixture record with a sufficiently long title",
		Symptom: &record.Symptom{
			Summary:         "a distinctive symptom for the usage wiring test",
			ErrorSignatures: []string{"zzdistinct-usage-signature-marker"},
		},
		Path: "experience/2026/0200-usage-fixture.md",
		Provenance: record.Provenance{
			Source: record.Source{Author: "test"}, RecordedAt: "2026-06-19",
			ValidatedAt: ptr("2026-06-19"), Valid: record.Validity{From: "2026-06-19"},
		},
	}
}

func ptr(s string) *string { return &s }

func newUsageHandlers(t *testing.T, recs ...*record.Record) (*handlers, *index.Index) {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, usageTestRepo); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	logger := quietLogger()
	h := &handlers{ix: ix, repo: usageTestRepo, logger: logger}
	h.usage = newUsageRecorder(ix, logger, fixedClock)
	return h, ix
}

func TestSearchRecordsUsage(t *testing.T) {
	h, ix := newUsageHandlers(t, usageFixture())
	_, out, err := h.search(context.Background(), nil, SearchArgs{Query: "zzdistinct-usage-signature-marker"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(out.Hits) == 0 || out.Hits[0].ID != "exp-0200" {
		t.Fatalf("expected the fixture as a hit, got %+v", out.Hits)
	}
	h.usage.flush()

	u, err := ix.Usage(context.Background(), "exp-0200")
	if err != nil {
		t.Fatal(err)
	}
	if u.Retrieved != 1 {
		t.Fatalf("retrieved = %d, want 1 (a served record's usage must advance)", u.Retrieved)
	}
	if u.LastHit == nil || *u.LastHit != "2026-06-19" {
		t.Fatalf("last_hit = %v, want 2026-06-19", u.LastHit)
	}
}

func TestGetRecordsUsage(t *testing.T) {
	h, ix := newUsageHandlers(t, usageFixture())
	if _, _, err := h.get(context.Background(), nil, GetArgs{ID: "exp-0200"}); err != nil {
		t.Fatalf("get: %v", err)
	}
	h.usage.flush()
	u, _ := ix.Usage(context.Background(), "exp-0200")
	if u.Retrieved != 1 {
		t.Fatalf("get retrieved = %d, want 1", u.Retrieved)
	}
}

func TestSearchEmptyResultRecordsNoUsage(t *testing.T) {
	h, ix := newUsageHandlers(t, usageFixture())
	_, out, err := h.search(context.Background(), nil, SearchArgs{Query: "nothing-here-matches-anything-zzz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Hits) != 0 {
		t.Fatalf("expected no hits, got %+v", out.Hits)
	}
	h.usage.flush()
	u, _ := ix.Usage(context.Background(), "exp-0200")
	if u.Retrieved != 0 {
		t.Fatalf("a miss must not bump usage; retrieved = %d", u.Retrieved)
	}
}

// fakeUsageStore records calls and can be primed to fail.
type fakeUsageStore struct {
	mu    sync.Mutex
	calls [][]string
	dates []string
	err   error
}

func (f *fakeUsageStore) RecordHits(_ context.Context, ids []string, date string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, ids)
	f.dates = append(f.dates, date)
	return f.err
}

func TestUsageRecorderAsyncAndDate(t *testing.T) {
	f := &fakeUsageStore{}
	r := newUsageRecorder(f, quietLogger(), fixedClock)
	r.record([]string{"exp-1", "exp-2"})
	r.record(nil)        // no-op
	r.record([]string{}) // no-op
	r.flush()

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 1 {
		t.Fatalf("got %d writes, want 1 (empty lists are no-ops)", len(f.calls))
	}
	if got := f.calls[0]; len(got) != 2 || got[0] != "exp-1" {
		t.Fatalf("recorded ids = %v, want [exp-1 exp-2]", got)
	}
	if f.dates[0] != "2026-06-19" {
		t.Fatalf("date = %q, want 2026-06-19 (UTC)", f.dates[0])
	}
}

func TestUsageRecorderSwallowsErrors(t *testing.T) {
	f := &fakeUsageStore{err: errors.New("db is closed")}
	r := newUsageRecorder(f, quietLogger(), fixedClock)
	r.record([]string{"exp-1"}) // a failing usage write must not block or crash
	r.flush()
}

func TestUsageRecorderNilSafe(t *testing.T) {
	var r *usageRecorder
	r.record([]string{"exp-1"}) // nil recorder is a no-op, not a panic
	r.flush()
}

func TestUsageRecorderOwnsSlice(t *testing.T) {
	f := &fakeUsageStore{}
	r := newUsageRecorder(f, quietLogger(), fixedClock)
	ids := []string{"exp-1"}
	r.record(ids)
	ids[0] = "mutated" // caller reuses the slice after record returns
	r.flush()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls[0][0] != "exp-1" {
		t.Fatalf("recorder did not copy the slice; saw %q", f.calls[0][0])
	}
}
