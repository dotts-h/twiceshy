// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/fingerprint"
	"github.com/dotts-h/twiceshy/internal/record"
)

const fingerprintTestRepo = "github.com/dotts-h/twiceshy"

// mkFingerprintRecord builds a minimal validated trap with the given signatures.
func mkFingerprintRecord(t *testing.T, num int, title, summary string, sigs []string) *record.Record {
	t.Helper()
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
	src += `applies_to:
  - ecosystem: Go
    package: example.com/fp
resolution:
  root_cause: "a cause"
  fix: "a fix"
guard: { repro: null, guarding_test: "TestFP" }
provenance:
  source: { author: "horia", session: null, pr: null }
  recorded_at: 2026-06-12
  validated_at: 2026-06-12
  valid: { from: 2026-06-12, until: null }
  superseded_by: null
---

Narrative.
`
	rec, err := record.Parse(fmt.Sprintf("experience/2026/%04d-rec.md", num), []byte(src))
	if err != nil {
		t.Fatalf("fixture record invalid: %v", err)
	}
	return rec
}

// M8: read back what insertRecord actually stored — not what the algorithm
// would produce if we recomputed it on the test side only. A scope/repo drift in
// insertRecord passes Search-based proxies but breaks Assess's exact-known path.
func TestStoredFingerprintsMatchAlgorithm(t *testing.T) {
	ctx := context.Background()
	const sig = "disk-backed fingerprint agreement sig 42"
	rec := mkFingerprintRecord(t, 8, "fp store trap", "fp summary", []string{sig})

	ix, err := Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(ctx, []*record.Record{rec}, fingerprintTestRepo); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		fingerprint.Generic(sig):                  "generic",
		fingerprint.App(fingerprintTestRepo, sig): "app",
	}

	rows, err := ix.db.QueryContext(ctx, "SELECT fp, scope FROM fingerprints WHERE record_id = ?", rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	got := map[string]string{}
	for rows.Next() {
		var fp, scope string
		if err := rows.Scan(&fp, &scope); err != nil {
			t.Fatal(err)
		}
		got[fp] = scope
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if len(got) != len(want) {
		t.Fatalf("fingerprint row count = %d, want %d; got=%v want=%v", len(got), len(want), got, want)
	}
	for fp, scope := range want {
		if got[fp] != scope {
			t.Errorf("stored fingerprint %q scope = %q, want %q (fp derived via algorithm, not a frozen hash)", fp, got[fp], scope)
		}
	}

	a, err := ix.Assess(ctx, Query{Text: sig, Repo: fingerprintTestRepo})
	if err != nil {
		t.Fatal(err)
	}
	if a.Novelty != NoveltyKnown {
		t.Errorf("exact signature Assess = %v, want NoveltyKnown; stored rows did not round-trip to query-time dedup", a.Novelty)
	}
}
