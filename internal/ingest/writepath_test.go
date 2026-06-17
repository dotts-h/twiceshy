// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

// M7: the full write-path loop end to end — propose (Prepare) → reload (Marshal
// the proposed record, re-Parse it as if it landed on disk via PR, Rebuild the
// index) → read (Get and an IncludeQuarantined Search find it). No prior test
// exercised propose → persist → reload → retrieve as one chain; a break in
// Marshal↔Parse↔Rebuild for a freshly-proposed record would slip through.
func TestWritePathProposeReloadRead(t *testing.T) {
	ctx := context.Background()
	const repo = "github.com/dotts-h/twiceshy"

	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(ctx, nil, repo); err != nil {
		t.Fatal(err)
	}

	id, err := ix.NextID(ctx) // exp-0001 on an empty corpus
	if err != nil {
		t.Fatal(err)
	}
	draft := ingest.Draft{
		Kind:       "trap",
		Title:      "Novel tapir trap in the connection layer",
		Symptom:    &record.Symptom{Summary: "a brand new tapir-shaped failure", ErrorSignatures: []string{"tapir-novel-signature-zzz"}},
		AppliesTo:  []record.AppliesTo{{Ecosystem: "Go", Package: "example.com/tapir"}},
		Resolution: &record.Resolution{RootCause: "a leaked connection", Fix: "close it on the retry path"},
		Body:       "How the tapir trap manifests and how to guard against it.",
	}
	meta := ingest.Meta{ID: id, Author: "claude", Now: "2026-06-12"}

	// Propose.
	out, err := ingest.Prepare(ctx, ix, repo, draft, meta)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Novelty == index.NoveltyKnown || out.Record == nil {
		t.Fatalf("a novel draft must yield a quarantined record, got novelty=%v record=%v", out.Novelty, out.Record)
	}
	if out.Record.Status != "quarantined" {
		t.Errorf("proposed record must be quarantined, got %q", out.Record.Status)
	}

	// Reload: marshal as it would be written, re-parse as it would be re-read.
	md, err := record.Marshal(out.Record)
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := record.Parse(out.Record.Path, md)
	if err != nil {
		t.Fatalf("the proposed record must re-parse from its on-disk form: %v\n%s", err, md)
	}
	if err := ix.Rebuild(ctx, []*record.Record{reparsed}, repo); err != nil {
		t.Fatalf("Rebuild with the landed record: %v", err)
	}

	// Read.
	got, err := ix.Get(ctx, id)
	if err != nil {
		t.Fatalf("the landed record must be retrievable by id: %v", err)
	}
	if got.Status != "quarantined" {
		t.Errorf("landed record status = %q, want quarantined", got.Status)
	}
	hits, err := ix.Search(ctx, index.Query{Text: "tapir-novel-signature-zzz", Floor: index.FloorOff, IncludeQuarantined: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hits {
		if h.ID == id {
			found = true
		}
	}
	if !found {
		t.Errorf("the landed record must be retrievable by its signature, got %+v", hits)
	}
}
