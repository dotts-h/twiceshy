// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

func TestUsageFlush_MaterializesAndIsIdempotent(t *testing.T) {
	dir := tempCorpus(t)
	rec := packFixture("0555", "validated", "MIT", "")
	writeFixture(t, dir, rec)

	db := filepath.Join(t.TempDir(), "ix.db")
	var indexOut bytes.Buffer
	if err := run(context.Background(), []string{"index", "-corpus", dir, "-db", db}, &indexOut, noEnv); err != nil {
		t.Fatalf("index: %v", err)
	}

	ctx := context.Background()
	ix, err := index.Open(db)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	const hits = 2
	for i := 0; i < hits; i++ {
		if err := ix.RecordHits(ctx, []string{rec.ID}, "2026-06-19"); err != nil {
			t.Fatalf("RecordHits: %v", err)
		}
	}
	if err := ix.ConfirmHelpful(ctx, rec.ID); err != nil {
		t.Fatalf("ConfirmHelpful: %v", err)
	}
	const pushes = 3
	for i := 0; i < pushes; i++ {
		if err := ix.RecordPushes(ctx, []string{rec.ID}); err != nil {
			t.Fatalf("RecordPushes: %v", err)
		}
	}
	if err := ix.Close(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runUsageFlush(ctx, []string{"-corpus", dir, "-db", db}, &out); err != nil {
		t.Fatalf("usage-flush: %v", err)
	}
	if !strings.Contains(out.String(), "updated 1 of 1") {
		t.Fatalf("first flush output = %q, want updated 1 of 1", out.String())
	}

	recs, err := record.LoadCorpus(dir)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("corpus len = %d, want 1", len(recs))
	}
	u := recs[0].Provenance.Usage
	if u == nil {
		t.Fatal("provenance.usage missing after flush")
	}
	if u.Retrieved != hits {
		t.Fatalf("retrieved = %d, want %d", u.Retrieved, hits)
	}
	if u.LastHit == nil || *u.LastHit != "2026-06-19" {
		t.Fatalf("last_hit = %v, want 2026-06-19", u.LastHit)
	}
	if u.ConfirmedHelpful != 1 {
		t.Fatalf("confirmed_helpful = %d, want 1", u.ConfirmedHelpful)
	}
	// pushed must materialize too — otherwise every Rebuild/hot-reload erases the
	// push impressions (the index usage table is wiped on a full reset).
	if u.Pushed != pushes {
		t.Fatalf("pushed = %d, want %d (push impressions must flush to provenance)", u.Pushed, pushes)
	}

	out.Reset()
	if err := runUsageFlush(ctx, []string{"-corpus", dir, "-db", db}, &out); err != nil {
		t.Fatalf("second usage-flush: %v", err)
	}
	if !strings.Contains(out.String(), "updated 0 of 1") {
		t.Fatalf("second flush output = %q, want updated 0 of 1", out.String())
	}
}

// Two records with DIFFERENT usage must each materialize their own counters —
// guards against a per-iteration pointer-aliasing regression in the flush loop.
func TestUsageFlush_MultipleRecordsNoAliasing(t *testing.T) {
	dir := tempCorpus(t)
	a := packFixture("0561", "validated", "MIT", "")
	b := packFixture("0562", "validated", "MIT", "")
	writeFixture(t, dir, a)
	writeFixture(t, dir, b)

	db := filepath.Join(t.TempDir(), "ix.db")
	if err := run(context.Background(), []string{"index", "-corpus", dir, "-db", db}, &bytes.Buffer{}, noEnv); err != nil {
		t.Fatalf("index: %v", err)
	}
	ctx := context.Background()
	ix, err := index.Open(db)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// a hit 3×, b hit 1× — distinct counters.
	for i := 0; i < 3; i++ {
		if err := ix.RecordHits(ctx, []string{a.ID}, "2026-06-19"); err != nil {
			t.Fatal(err)
		}
	}
	if err := ix.RecordHits(ctx, []string{b.ID}, "2026-06-18"); err != nil {
		t.Fatal(err)
	}
	if err := ix.Close(); err != nil {
		t.Fatal(err)
	}

	if err := runUsageFlush(ctx, []string{"-corpus", dir, "-db", db}, &bytes.Buffer{}); err != nil {
		t.Fatalf("usage-flush: %v", err)
	}

	recs, err := record.LoadCorpus(dir)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	got := map[string]record.Usage{}
	for _, r := range recs {
		if r.Provenance.Usage != nil {
			got[r.ID] = *r.Provenance.Usage
		}
	}
	if got[a.ID].Retrieved != 3 || got[a.ID].LastHit == nil || *got[a.ID].LastHit != "2026-06-19" {
		t.Errorf("record A usage = %+v, want retrieved 3 / last_hit 2026-06-19", got[a.ID])
	}
	if got[b.ID].Retrieved != 1 || got[b.ID].LastHit == nil || *got[b.ID].LastHit != "2026-06-18" {
		t.Errorf("record B usage = %+v, want retrieved 1 / last_hit 2026-06-18", got[b.ID])
	}
}
