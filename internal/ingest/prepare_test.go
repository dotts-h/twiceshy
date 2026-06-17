package ingest_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

// The orchestrator's gate for ingest.Prepare: the dedup-at-ingest core of the
// write path. An agent-proposed draft is classified against the corpus via
// index.Assess (known / similar / novel), an exact match is NOT re-recorded,
// and everything that IS recorded is forced to status quarantined with filled
// provenance and a schema-valid Record — git/PR is the trust boundary, so a
// fresh record is never born validated.

const repo = "github.com/dotts-h/twiceshy"

func strptr(s string) *string { return &s }

// mkRec builds a validated trap fixture with one error signature.
func mkRec(t *testing.T, num, title, summary, sig string) *record.Record {
	t.Helper()
	src := fmt.Sprintf(`---
schema_version: 1
id: exp-%s
kind: trap
status: validated
title: %q
symptom:
  summary: %q
  error_signatures:
    - %q
applies_to:
  - ecosystem: Go
    package: example.com/db
resolution:
  root_cause: "a cause"
  fix: "a fix"
guard: { repro: null, guarding_test: "TestThing" }
provenance:
  source: { author: "horia", session: null, pr: null }
  recorded_at: 2026-06-12
  validated_at: 2026-06-12
  valid: { from: 2026-06-12, until: null }
  superseded_by: null
---

Narrative for %s.
`, num, title, summary, sig, title)
	rec, err := record.Parse(fmt.Sprintf("experience/2026/%s-rec.md", num), []byte(src))
	if err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}
	return rec
}

func openIx(t *testing.T, recs ...*record.Record) *index.Index {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, repo); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	return ix
}

// A trap draft modeling a real agent proposal.
func trapDraft(summary, sig string) ingest.Draft {
	return ingest.Draft{
		Kind:       "trap",
		Title:      "Connection pool exhaustion under concurrent load",
		Symptom:    &record.Symptom{Summary: summary, ErrorSignatures: []string{sig}},
		AppliesTo:  []record.AppliesTo{{Ecosystem: "Go", Package: "example.com/db"}},
		Resolution: &record.Resolution{RootCause: "leaked connections", Fix: "defer Close"},
		Guard:      &record.Guard{GuardingTest: strptr("TestPoolGuard")},
		Body:       "How the pool runs dry and how to guard it.",
	}
}

func meta() ingest.Meta {
	return ingest.Meta{ID: "exp-0042", Author: "claude", Session: strptr("sess-xyz"), Now: "2026-06-17"}
}

// A genuinely new draft becomes a quarantined, schema-valid record with filled
// provenance — never born validated.
func TestPrepare_NovelQuarantines(t *testing.T) {
	ix := openIx(t, mkRec(t, "0001", "Pool dries up under load", "the pool runs dry under load",
		"database connection pool exhausted under load"))
	out, err := ingest.Prepare(context.Background(), ix, repo,
		trapDraft("a totally unrelated zorblefrag fault", "zorblefrag quux frobnicator imploded"), meta())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Novelty != index.NoveltyNovel {
		t.Fatalf("want Novel, got %q", out.Novelty)
	}
	r := out.Record
	if r == nil {
		t.Fatal("Novel must produce a record to quarantine")
	}
	if r.Status != "quarantined" {
		t.Errorf("status must be forced to quarantined, got %q", r.Status)
	}
	if r.Provenance.ValidatedAt != nil {
		t.Errorf("a fresh record must not be validated, got validated_at=%v", *r.Provenance.ValidatedAt)
	}
	if r.Provenance.RecordedAt != "2026-06-17" || r.Provenance.Valid.From != "2026-06-17" {
		t.Errorf("provenance dates not filled from Meta.Now: %+v", r.Provenance)
	}
	if r.Provenance.Source.Author != "claude" || r.Provenance.Source.Session == nil || *r.Provenance.Source.Session != "sess-xyz" {
		t.Errorf("provenance source not filled from Meta: %+v", r.Provenance.Source)
	}
	if err := record.Validate(r); err != nil {
		t.Errorf("prepared record must be schema-valid: %v", err)
	}
	if len(out.Candidates) != 0 {
		t.Errorf("Novel carries no candidates, got %+v", out.Candidates)
	}
}

// An exact-signature match to an existing record is NOT re-recorded: Known,
// no new record, the existing one returned as evidence.
func TestPrepare_KnownIsNotDuplicated(t *testing.T) {
	sig := "database connection pool exhausted under load"
	ix := openIx(t, mkRec(t, "0001", "Pool dries up under load", "the pool runs dry", sig))
	out, err := ingest.Prepare(context.Background(), ix, repo,
		trapDraft("the pool runs dry", sig), meta())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Novelty != index.NoveltyKnown {
		t.Fatalf("want Known, got %q", out.Novelty)
	}
	if out.Record != nil {
		t.Errorf("Known must NOT create a duplicate record, got %+v", out.Record)
	}
	if len(out.Candidates) == 0 {
		t.Errorf("Known must return the existing record as evidence")
	}
}

// An exact duplicate whose shared signature is NOT first must still be caught:
// the index fingerprints every signature, so the probe must too.
func TestPrepare_KnownViaNonFirstSignature(t *testing.T) {
	shared := "database connection pool exhausted under load"
	ix := openIx(t, mkRec(t, "0001", "Pool dries up under load", "the pool runs dry", shared))
	d := trapDraft("a differently-worded summary", "some-unique-leading-signature")
	d.Symptom.ErrorSignatures = []string{"some-unique-leading-signature", shared} // shared is second
	out, err := ingest.Prepare(context.Background(), ix, repo, d, meta())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Novelty != index.NoveltyKnown {
		t.Fatalf("a shared non-first signature must be Known, got %q", out.Novelty)
	}
	if out.Record != nil {
		t.Errorf("Known must not duplicate, got %+v", out.Record)
	}
}

// A malformed Meta.Now must produce an error, never a panic.
func TestPrepare_MalformedNowErrors(t *testing.T) {
	ix := openIx(t)
	m := meta()
	m.Now = "" // shorter than a YYYY year
	_, err := ingest.Prepare(context.Background(), ix, repo,
		trapDraft("fresh", "yet-another-unique-signature-q"), m)
	if err == nil {
		t.Fatal("malformed Now must return an error, not panic")
	}
}

// A near-but-distinct draft (lexical overlap, no exact fingerprint) is still
// recorded — quarantined — but carries the leads to verify against.
func TestPrepare_SimilarQuarantinesWithLeads(t *testing.T) {
	ix := openIx(t, mkRec(t, "0001", "Pool dries up under load", "the pool runs dry",
		"database connection pool exhausted under load"))
	out, err := ingest.Prepare(context.Background(), ix, repo,
		trapDraft("pool dry during migration", "database connection pool exhausted during migration"), meta())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Novelty != index.NoveltySimilar {
		t.Fatalf("want Similar, got %q", out.Novelty)
	}
	if out.Record == nil || out.Record.Status != "quarantined" {
		t.Fatalf("Similar must still quarantine a record, got %+v", out.Record)
	}
	if len(out.Candidates) == 0 {
		t.Errorf("Similar must return leads to verify against")
	}
}

// Path is derived deterministically from the id and Meta.Now and is valid.
func TestPrepare_PathDerivedFromIDAndDate(t *testing.T) {
	ix := openIx(t)
	out, err := ingest.Prepare(context.Background(), ix, repo,
		trapDraft("brand new fault", "unique-snowflake-signature-12345"), meta())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	want := "experience/2026/0042-connection-pool-exhaustion-under-concurrent-load.md"
	if out.Record.Path != want {
		t.Errorf("path = %q, want %q", out.Record.Path, want)
	}
}

// An invalid draft (trap without a resolution) must error, not silently
// quarantine malformed state.
func TestPrepare_InvalidDraftErrors(t *testing.T) {
	ix := openIx(t)
	d := trapDraft("missing resolution", "some-unique-signature-zzz")
	d.Resolution = nil
	_, err := ingest.Prepare(context.Background(), ix, repo, d, meta())
	if err == nil {
		t.Fatal("invalid draft must return an error, not a quarantined record")
	}
}

// An empty narrative body is rejected (record invariant the write path enforces).
func TestPrepare_EmptyBodyErrors(t *testing.T) {
	ix := openIx(t)
	d := trapDraft("no body", "another-unique-signature-yyy")
	d.Body = "  "
	_, err := ingest.Prepare(context.Background(), ix, repo, d, meta())
	if err == nil {
		t.Fatal("empty body must return an error")
	}
}
