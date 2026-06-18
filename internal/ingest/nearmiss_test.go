// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
)

// The near-miss guard (corpus-bootstrap.md §8): importing must never trade
// precision for volume. Two invariants protect that, and they are already
// enforced by existing mechanisms — these tests lock them so a future importer
// change cannot silently regress them:
//
//  1. Every imported record is born `quarantined`, so it is pull-only and never
//     reaches the push channel (Prepare forces the status). A quarantined record
//     is below the push threshold by construction.
//  2. The importer emits at most one record per distinct primary error signature
//     (the (package, breaking-change) cap) — Prepare's dedup probes each error
//     signature and any Known hit is terminal, and the CLI batch dedup keys on
//     the same primary signal. We assert no two drafts from a source share a
//     primary error signature, so the cap holds before dedup even runs.

func primarySignal(d ingest.Draft) string {
	if d.Symptom != nil {
		for _, s := range d.Symptom.ErrorSignatures {
			if s != "" {
				return s
			}
		}
	}
	return d.Title
}

func TestImportedRecordsAreBornQuarantined(t *testing.T) {
	ix := newEmptyIndex(t)
	for _, src := range []ingest.Source{ingest.NewGoSource(), ingest.NewOSVSource()} {
		drafts, err := src.Drafts(context.Background())
		if err != nil {
			t.Fatalf("%s.Drafts: %v", src.Name(), err)
		}
		for i, d := range drafts {
			out, err := ingest.Prepare(context.Background(), ix, "", d,
				ingest.Meta{ID: "exp-9000", Author: "twiceshy-importer", Now: "2026-06-18"})
			if err != nil {
				t.Fatalf("%s draft %d: Prepare: %v", src.Name(), i, err)
			}
			if out.Record == nil {
				continue // deduped against an earlier identical probe — fine
			}
			if out.Record.Status != "quarantined" {
				t.Errorf("%s draft %d: status = %q, want quarantined (pull-only)", src.Name(), i, out.Record.Status)
			}
		}
	}
}

func TestSourceDraftsHaveDistinctPrimarySignals(t *testing.T) {
	for _, src := range []ingest.Source{ingest.NewGoSource(), ingest.NewOSVSource()} {
		drafts, err := src.Drafts(context.Background())
		if err != nil {
			t.Fatalf("%s.Drafts: %v", src.Name(), err)
		}
		seen := map[string]bool{}
		for _, d := range drafts {
			k := primarySignal(d)
			if seen[k] {
				t.Errorf("%s emits two drafts with the same primary signal %q — violates the one-record-per-(package,breaking-change) cap", src.Name(), k)
			}
			seen[k] = true
		}
	}
}

// newEmptyIndex opens an in-memory-equivalent index over an empty corpus.
func newEmptyIndex(t *testing.T) *index.Index {
	t.Helper()
	ix, err := index.Open(t.TempDir() + "/ix.db")
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), nil, ""); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	return ix
}
