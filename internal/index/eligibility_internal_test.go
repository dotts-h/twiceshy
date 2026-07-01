// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// eligRec builds a minimal in-memory validated record for eligibleDF tests.
// Rebuild does not validate records (that's record.Parse/Validate's job), so a
// bare struct with just the fields insertRecord reads is enough (usageRec in
// usage_test.go does the same for the sibling usage tests).
func eligRec(id, kind, author, text string) *record.Record {
	return &record.Record{
		SchemaVersion: 1, ID: id, Kind: kind, Status: "validated",
		Title: text + " — a fixture title long enough to pass validation",
		Path:  "experience/2026/" + id[4:] + "-x.md",
		Provenance: record.Provenance{
			Source:     record.Source{Author: author},
			RecordedAt: "2026-06-19",
			Valid:      record.Validity{From: "2026-06-19"},
		},
	}
}

// TestEligibleDFRestrictsToKindAndOrigin is the direct unit test behind #0107:
// eligibleDF must count only validated records of an eligible kind (trap/fix)
// from a non-importer origin, while validatedDF (unchanged) counts all three.
func TestEligibleDFRestrictsToKindAndOrigin(t *testing.T) {
	ctx := context.Background()
	recs := []*record.Record{
		eligRec("exp-0300", "trap", "twiceshy-importer", "an importer-origin trap about zorbnaxos"),
		eligRec("exp-0301", "convention", "horia", "a convention about zorbnaxos naming"),
		eligRec("exp-0302", "trap", "horia", "an eligible trap handling zorbnaxos overload"),
	}
	ix, err := Open(filepath.Join(t.TempDir(), "elig.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(ctx, recs, ""); err != nil {
		t.Fatal(err)
	}

	if vdf, err := ix.validatedDF(ctx, "zorbnaxos"); err != nil {
		t.Fatal(err)
	} else if vdf != 3 {
		t.Fatalf("validatedDF(zorbnaxos) = %d, want 3 (unaffected by eligibility)", vdf)
	}

	edf, err := ix.eligibleDF(ctx, "zorbnaxos")
	if err != nil {
		t.Fatal(err)
	}
	if edf != 1 {
		t.Fatalf("eligibleDF(zorbnaxos) = %d, want 1 (only exp-0302 is trap/fix + non-importer)", edf)
	}
}

// TestEligibleDFZeroWhenOnlyImporterOriginCarriesToken is the acceptance case
// named in #0106/#0107: a token living ONLY in importer-origin advisory material
// must never be discriminative, however rare it is corpus-wide.
func TestEligibleDFZeroWhenOnlyImporterOriginCarriesToken(t *testing.T) {
	ctx := context.Background()
	rec := eligRec("exp-0310", "trap", "twiceshy-importer", "flimbosaur advisory cve-0000-0000")
	ix, err := Open(filepath.Join(t.TempDir(), "elig2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(ctx, []*record.Record{rec}, ""); err != nil {
		t.Fatal(err)
	}

	if df, err := ix.eligibleDF(ctx, "flimbosaur"); err != nil {
		t.Fatal(err)
	} else if df != 0 {
		t.Fatalf("eligibleDF(flimbosaur) = %d, want 0 (only an importer-origin record carries it)", df)
	}

	disc, err := ix.discriminativeTokens(ctx, "flimbosaur")
	if err != nil {
		t.Fatal(err)
	}
	if len(disc) != 0 {
		t.Fatalf("discriminativeTokens = %v, want empty — importer-only token must never open the gate", disc)
	}
}
