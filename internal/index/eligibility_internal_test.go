// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"path/filepath"
	"reflect"
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

	disc, _, err := ix.discriminativeTokens(ctx, "flimbosaur")
	if err != nil {
		t.Fatal(err)
	}
	if len(disc) != 0 {
		t.Fatalf("discriminativeTokens = %v, want empty — importer-only token must never open the gate", disc)
	}
}

// TestDiscriminativeTokens_SurfacesIdfFilteredCount pins the threading of the
// idfFiltered int result from discriminativeTokensVia through its sole caller
// discriminativeTokens: discriminativeTokens must now return
// ([]string, int, error), with the count passed straight through unmodified
// from the df-injected helper (ix.eligibleDF) — no recomputation, no
// transformation. Two non-trivially-related cases triangulate this so a stub
// that always returns one constant (e.g. always idfFiltered=0) cannot satisfy
// the table:
//
//  1. default seam (the shipped-empty, unavailable embedded idf table):
//     nothing is globally filtered, so discriminativeTokens' count is 0 —
//     default-seam behavior is unaffected by this threading change.
//  2. an injected idf table makes exactly one otherwise-eligible token
//     globally common: discriminativeTokens' count must be 1, and must equal
//     — exactly, not just numerically by coincidence — whatever
//     discriminativeTokensVia(ctx, text, ix.eligibleDF) itself produces for
//     the same input, proving the caller passes the helper's count straight
//     through rather than recomputing or dropping it.
func TestDiscriminativeTokens_SurfacesIdfFilteredCount(t *testing.T) {
	ctx := context.Background()
	recs := []*record.Record{
		eligRec("exp-0320", "trap", "horia", "a rare trap about zorbnaxos overload"),
		eligRec("exp-0321", "trap", "horia", "a rare trap about flimbosaur overload"),
	}
	ix, err := Open(filepath.Join(t.TempDir(), "elig3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(ctx, recs, ""); err != nil {
		t.Fatal(err)
	}

	const text = "zorbnaxos flimbosaur"

	t.Run("default seam: nothing globally filtered, idfFiltered=0", func(t *testing.T) {
		tokens, idfFiltered, err := ix.discriminativeTokens(ctx, text)
		if err != nil {
			t.Fatalf("discriminativeTokens: %v", err)
		}
		if idfFiltered != 0 {
			t.Fatalf("idfFiltered = %d, want 0 (default seam filters nothing)", idfFiltered)
		}
		wantTokens := []string{"zorbnaxos", "flimbosaur"}
		if !reflect.DeepEqual(tokens, wantTokens) {
			t.Fatalf("tokens = %v, want %v", tokens, wantTokens)
		}
	})

	t.Run("injected table: idfFiltered threads straight through from discriminativeTokensVia", func(t *testing.T) {
		orig := idfProvider
		defer func() { idfProvider = orig }()
		idfProvider = fakeIDFProvider{
			available: true,
			totalDocs: 100,
			df:        map[string]uint64{"flimbosaur": 50}, // 50/100 = 0.50 > 0.10 ceiling
		}

		wantTokens, wantIdfFiltered, err := ix.discriminativeTokensVia(ctx, text, ix.eligibleDF)
		if err != nil {
			t.Fatalf("discriminativeTokensVia: %v", err)
		}

		gotTokens, gotIdfFiltered, err := ix.discriminativeTokens(ctx, text)
		if err != nil {
			t.Fatalf("discriminativeTokens: %v", err)
		}

		if gotIdfFiltered != wantIdfFiltered {
			t.Fatalf("discriminativeTokens idfFiltered = %d, want %d (must match discriminativeTokensVia exactly — straight pass-through)", gotIdfFiltered, wantIdfFiltered)
		}
		if gotIdfFiltered != 1 {
			t.Fatalf("idfFiltered = %d, want 1 (flimbosaur is globally common)", gotIdfFiltered)
		}
		if !reflect.DeepEqual(gotTokens, wantTokens) {
			t.Fatalf("tokens = %v, want %v", gotTokens, wantTokens)
		}
	})
}
