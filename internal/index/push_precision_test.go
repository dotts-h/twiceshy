// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"path/filepath"
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
	disc, err := ix.discriminativeTokens(ctx, `fts5 syntax error near "."`)
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
