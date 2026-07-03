// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/testcorpus"
)

// TestPushGateDiscriminativeTokensOnFixture exercises discriminativeTokens and
// validatedDF against the fixture corpus. It verifies that a known-distinctive
// error token (fts5) is discriminative on the fixture corpus, and that the
// df lookup returns a non-zero count for a word present in validated records.
// This preserves coverage of discriminativeTokens/validatedDF without the live
// corpus (TestPushGateExcludesCommonVocabulary in push_precision_livecorpus_test.go
// runs the full adversarial-vocab check against the live corpus).
func TestPushGateDiscriminativeTokensOnFixture(t *testing.T) {
	ctx := context.Background()
	recs, err := record.LoadCorpus(testcorpus.Root())
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	ix, err := Open(filepath.Join(t.TempDir(), "fixture-vocab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(ctx, recs, ""); err != nil {
		t.Fatal(err)
	}

	// "fts5" appears in exp-0001/exp-0002 validated records — it must be discriminative.
	disc, _, err := ix.discriminativeTokens(ctx, `fts5 syntax error near "."`)
	if err != nil {
		t.Fatalf("discriminativeTokens: %v", err)
	}
	if len(disc) == 0 {
		t.Error("fts5 query must yield at least one discriminative token on the fixture corpus")
	}

	// validatedDF must return a non-zero count for a word present in validated records.
	df, err := ix.validatedDF(ctx, "fts5")
	if err != nil {
		t.Fatalf("validatedDF: %v", err)
	}
	if df == 0 {
		t.Error("validatedDF(fts5) must be > 0 on the fixture corpus")
	}
}

// TestPushGateIdfFilteredCount pins the threading of the idf-filtered count —
// "how many otherwise-eligible tokens did the global-IDF check (globallyCommonWord,
// ADR-0017) drop" — into the new PushDecision.IdfFiltered field, on the two
// RetrievePushTraced return paths that actually compute discriminative tokens:
// the corroboration-precondition early return (len(disc) < 2, #0108) and the
// final served-hits return. The two subtests inject DIFFERENT fake idf tables
// that filter a DIFFERENT number of tokens (1 vs 2) and land on DIFFERENT
// return paths, so a stub that always reports one constant IdfFiltered value —
// or that only wires one of the two return paths — cannot satisfy both.
func TestPushGateIdfFilteredCount(t *testing.T) {
	ctx := context.Background()

	t.Run("corroboration-precondition early return reports IdfFiltered", func(t *testing.T) {
		rec := eligRec("exp-0330", "trap", "horia", "a rare trap about quixolite and vantabrex overload")
		ix, err := Open(filepath.Join(t.TempDir(), "gate1.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ix.Close() })
		if err := ix.Rebuild(ctx, []*record.Record{rec}, ""); err != nil {
			t.Fatal(err)
		}

		orig := idfProvider
		defer func() { idfProvider = orig }()
		idfProvider = fakeIDFProvider{
			available: true,
			totalDocs: 100,
			// vantabrex is globally common (50/100 = 0.50 > 0.10 ceiling); quixolite
			// is absent from the table, so it is left untouched.
			df: map[string]uint64{"vantabrex": 50},
		}

		dec, err := ix.RetrievePushTraced(ctx, Query{Text: "quixolite vantabrex"})
		if err != nil {
			t.Fatalf("RetrievePushTraced: %v", err)
		}
		if dec.IdfFiltered != 1 {
			t.Errorf("IdfFiltered = %d, want 1 (vantabrex dropped; quixolite alone is < 2 tokens -> corroboration-precondition return)", dec.IdfFiltered)
		}
		if !reflect.DeepEqual(dec.Discriminative, []string{"quixolite"}) {
			t.Errorf("Discriminative = %v, want [quixolite] — must be unaffected by adding IdfFiltered", dec.Discriminative)
		}
		if len(dec.Served) != 0 {
			t.Errorf("Served = %v, want empty — the corroboration precondition must still block a single token", dec.Served)
		}
		if dec.FingerprintBypass {
			t.Error("FingerprintBypass = true, want false — no fingerprint match in this fixture")
		}
	})

	t.Run("final served-hits return reports IdfFiltered", func(t *testing.T) {
		rec := eligRec("exp-0331", "trap", "horia", "a rare trap about wobbuffet grimtusk snarlfen overload")
		ix, err := Open(filepath.Join(t.TempDir(), "gate2.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ix.Close() })
		if err := ix.Rebuild(ctx, []*record.Record{rec}, ""); err != nil {
			t.Fatal(err)
		}

		orig := idfProvider
		defer func() { idfProvider = orig }()
		idfProvider = fakeIDFProvider{
			available: true,
			totalDocs: 100,
			// grimtusk and snarlfen are globally common; wobbuffet is absent from
			// the table and survives.
			df: map[string]uint64{"grimtusk": 60, "snarlfen": 70},
		}

		// ErrorTrigger bypasses the two-token corroboration precondition (#0108), so
		// the single surviving token still reaches the final served-hits return —
		// isolating this subtest to that return path, distinct from the one above.
		q := Query{Text: "wobbuffet grimtusk snarlfen", ErrorTrigger: true}
		dec, err := ix.RetrievePushTraced(ctx, q)
		if err != nil {
			t.Fatalf("RetrievePushTraced: %v", err)
		}
		if dec.IdfFiltered != 2 {
			t.Errorf("IdfFiltered = %d, want 2 (grimtusk + snarlfen dropped)", dec.IdfFiltered)
		}
		if !reflect.DeepEqual(dec.Discriminative, []string{"wobbuffet"}) {
			t.Errorf("Discriminative = %v, want [wobbuffet] — must be unaffected by adding IdfFiltered", dec.Discriminative)
		}
		if dec.FingerprintBypass {
			t.Error("FingerprintBypass = true, want false — no fingerprint match in this fixture")
		}

		// The trace must still serve EXACTLY what RetrievePush serves — adding
		// IdfFiltered must not perturb Served (no wrapper drift).
		plain, err := ix.RetrievePush(ctx, q)
		if err != nil {
			t.Fatalf("RetrievePush: %v", err)
		}
		if len(plain) != len(dec.Served) {
			t.Errorf("wrapper drift: RetrievePush served %d, traced served %d", len(plain), len(dec.Served))
		}
	})
}
