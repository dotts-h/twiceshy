// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

// BatchKey is the within-batch dedup signal shared by every batch caller
// (importer, retro-intake, intake-reports) — it must never drift between them.
func TestBatchKey(t *testing.T) {
	cases := []struct {
		name string
		d    ingest.Draft
		want string
	}{
		{
			"first non-blank signature wins",
			ingest.Draft{Title: "t", Symptom: &record.Symptom{ErrorSignatures: []string{"  ", "sig-1", "sig-2"}}},
			"sig-1",
		},
		{
			"all-blank signatures fall back to title",
			ingest.Draft{Title: "t", Symptom: &record.Symptom{ErrorSignatures: []string{"  ", ""}}},
			"t",
		},
		{
			"no symptom falls back to title",
			ingest.Draft{Title: "t"},
			"t",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ingest.BatchKey(tc.d); got != tc.want {
				t.Errorf("BatchKey() = %q, want %q", got, tc.want)
			}
		})
	}
}

// BumpID advances by exactly one and never truncates a digit-width rollover.
func TestBumpID(t *testing.T) {
	cases := []struct{ id, want string }{
		{"exp-0001", "exp-0002"},
		{"exp-9999", "exp-10000"}, // 4-digit -> 5-digit boundary
		{"not-an-id", "exp-0001"}, // unparseable id treated as 0, bumps to 1
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if got := ingest.BumpID(tc.id); got != tc.want {
				t.Errorf("BumpID(%q) = %q, want %q", tc.id, got, tc.want)
			}
		})
	}
}

func noopPersist(string, *record.Record) error { return nil }

// Two novel drafts are created, persisted, and the id is bumped between them.
func TestImportBatch_CreatesAndBumpsID(t *testing.T) {
	ix := openIx(t)
	var persisted []string
	persist := func(_ string, rec *record.Record) error { persisted = append(persisted, rec.ID); return nil }
	var out bytes.Buffer

	drafts := []ingest.Draft{
		trapDraft("first fault", "zorblefrag-one"),
		trapDraft("second fault", "zorblefrag-two"),
	}
	st, err := ingest.ImportBatch(context.Background(), ix, repo, "/corpus", "test-source",
		drafts, "exp-0001", "claude", "2026-06-17", false, 0, persist, &out)
	if err != nil {
		t.Fatalf("ImportBatch: %v", err)
	}
	if st.Created != 2 || st.Skipped != 0 || st.Flagged != 0 {
		t.Fatalf("stats = %+v, want {Created:2 Skipped:0 Flagged:0}", st)
	}
	if len(persisted) != 2 || persisted[0] != "exp-0001" || persisted[1] != "exp-0002" {
		t.Fatalf("persisted = %v, want [exp-0001 exp-0002] (id bumped between records)", persisted)
	}
}

// Two drafts sharing the same BatchKey within one batch: only the first is
// created; the second is skipped WITHOUT ever reaching the corpus-dedup probe.
func TestImportBatch_DedupsWithinBatchByKey(t *testing.T) {
	ix := openIx(t)
	calls := 0
	persist := func(_ string, _ *record.Record) error { calls++; return nil }
	var out bytes.Buffer

	sig := "shared-signature-within-batch"
	drafts := []ingest.Draft{trapDraft("first", sig), trapDraft("second", sig)}
	st, err := ingest.ImportBatch(context.Background(), ix, repo, "/corpus", "test-source",
		drafts, "exp-0001", "claude", "2026-06-17", false, 0, persist, &out)
	if err != nil {
		t.Fatalf("ImportBatch: %v", err)
	}
	if st.Created != 1 || st.Skipped != 1 {
		t.Fatalf("stats = %+v, want {Created:1 Skipped:1}", st)
	}
	if calls != 1 {
		t.Fatalf("persist called %d times, want 1", calls)
	}
}

// A draft that is already Known in the corpus (exact signature match) is
// skipped, not re-created.
func TestImportBatch_SkipsKnownAgainstCorpus(t *testing.T) {
	sig := "already-in-the-corpus"
	ix := openIx(t, mkRec(t, "0001", "Existing record", "existing summary", sig))
	calls := 0
	persist := func(_ string, _ *record.Record) error { calls++; return nil }
	var out bytes.Buffer

	st, err := ingest.ImportBatch(context.Background(), ix, repo, "/corpus", "test-source",
		[]ingest.Draft{trapDraft("dup", sig)}, "exp-0002", "claude", "2026-06-17", false, 0, persist, &out)
	if err != nil {
		t.Fatalf("ImportBatch: %v", err)
	}
	if st.Created != 0 || st.Skipped != 1 {
		t.Fatalf("stats = %+v, want {Created:0 Skipped:1}", st)
	}
	if calls != 0 {
		t.Fatalf("persist must not be called for a known duplicate, called %d times", calls)
	}
}

// A limit > 0 stops the batch after that many creations, even with more novel
// drafts remaining — bounding a scheduled import so it grows gradually (#0022).
func TestImportBatch_StopsAtLimit(t *testing.T) {
	ix := openIx(t)
	var persisted []string
	persist := func(_ string, rec *record.Record) error { persisted = append(persisted, rec.ID); return nil }
	var out bytes.Buffer

	drafts := []ingest.Draft{
		trapDraft("one", "limit-sig-one"),
		trapDraft("two", "limit-sig-two"),
		trapDraft("three", "limit-sig-three"),
	}
	st, err := ingest.ImportBatch(context.Background(), ix, repo, "/corpus", "test-source",
		drafts, "exp-0001", "claude", "2026-06-17", false, 2, persist, &out)
	if err != nil {
		t.Fatalf("ImportBatch: %v", err)
	}
	if st.Created != 2 {
		t.Fatalf("Created = %d, want 2 (limit must stop the batch early)", st.Created)
	}
	if len(persisted) != 2 {
		t.Fatalf("persisted %d records, want exactly 2", len(persisted))
	}
}

// dryRun counts what WOULD be created but never calls persist.
func TestImportBatch_DryRunDoesNotPersist(t *testing.T) {
	ix := openIx(t)
	calls := 0
	persist := func(_ string, _ *record.Record) error { calls++; return nil }
	var out bytes.Buffer

	st, err := ingest.ImportBatch(context.Background(), ix, repo, "/corpus", "test-source",
		[]ingest.Draft{trapDraft("dry", "dry-run-sig")}, "exp-0001", "claude", "2026-06-17", true, 0, persist, &out)
	if err != nil {
		t.Fatalf("ImportBatch: %v", err)
	}
	if st.Created != 1 {
		t.Fatalf("Created = %d, want 1 (dry-run still counts)", st.Created)
	}
	if calls != 0 {
		t.Fatalf("persist called %d times, want 0 under dry-run", calls)
	}
	if !bytes.Contains(out.Bytes(), []byte("would create")) {
		t.Errorf("output = %q, want it to say 'would create'", out.String())
	}
}

// A record whose narrative trips the safety screen is still created
// (quarantine-with-flag is the default policy) and counted as Flagged.
func TestImportBatch_CountsFlaggedRecords(t *testing.T) {
	ix := openIx(t)
	persist := noopPersist
	var out bytes.Buffer

	st, err := ingest.ImportBatch(context.Background(), ix, repo, "/corpus", "test-source",
		[]ingest.Draft{secretDraft()}, "exp-0001", "claude", "2026-06-17", false, 0, persist, &out)
	if err != nil {
		t.Fatalf("ImportBatch: %v", err)
	}
	if st.Created != 1 || st.Flagged != 1 {
		t.Fatalf("stats = %+v, want {Created:1 Flagged:1}", st)
	}
	if !bytes.Contains(out.Bytes(), []byte("FLAGGED")) {
		t.Errorf("output = %q, want a FLAGGED annotation", out.String())
	}
}

// A single draft that fails schema validation (here: a too-short title, the
// wtfjs "baNaNa" case, #0134) is a deterministic per-entry data defect — it
// must be skipped, counted, and logged, NOT abort the whole batch. The valid
// drafts around it still import. Mirrors the #0142 per-item-defect rule.
func TestImportBatch_SkipsInvalidDraftAndContinues(t *testing.T) {
	ix := openIx(t)
	var persisted []string
	persist := func(_ string, rec *record.Record) error { persisted = append(persisted, rec.ID); return nil }
	var out bytes.Buffer

	bad := trapDraft("too short a title fails validation", "banana-coercion-sig")
	bad.Title = "baNaNa" // 6 chars — outside record.Validate's 8..120 title bound
	drafts := []ingest.Draft{
		trapDraft("first ok", "zorblefrag-uno"),
		bad,
		trapDraft("second ok", "zorblefrag-dos"),
	}
	st, err := ingest.ImportBatch(context.Background(), ix, repo, "/corpus", "test-source",
		drafts, "exp-0001", "claude", "2026-06-17", false, 0, persist, &out)
	if err != nil {
		t.Fatalf("ImportBatch must not abort on one invalid draft: %v", err)
	}
	if st.Created != 2 || st.Invalid != 1 {
		t.Fatalf("stats = %+v, want {Created:2 Invalid:1}", st)
	}
	if len(persisted) != 2 || persisted[0] != "exp-0001" || persisted[1] != "exp-0002" {
		t.Fatalf("persisted = %v, want [exp-0001 exp-0002] (id not consumed by the skipped invalid draft)", persisted)
	}
	if !bytes.Contains(out.Bytes(), []byte("baNaNa")) || !bytes.Contains(out.Bytes(), []byte("invalid")) {
		t.Errorf("output = %q, want a logged 'invalid' skip naming baNaNa (no silent drop)", out.String())
	}
}

// An invalid-draft skip is scoped to record.Validate failures (ErrInvalidDraft);
// this pins that the sentinel actually wraps that path so ImportBatch can key on it.
func TestPrepare_InvalidDraftIsSentinel(t *testing.T) {
	ix := openIx(t)
	bad := trapDraft("short title", "sentinel-sig")
	bad.Title = "tiny"
	_, err := ingest.Prepare(context.Background(), ix, repo, bad, meta())
	if err == nil {
		t.Fatal("Prepare must reject a too-short title")
	}
	if !errors.Is(err, ingest.ErrInvalidDraft) {
		t.Errorf("err = %v, want it to wrap ingest.ErrInvalidDraft", err)
	}
}
