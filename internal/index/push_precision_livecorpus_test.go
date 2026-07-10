//go:build livecorpus

// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// TestPushGateExcludesCommonVocabulary is the mechanical precision guard: over the
// LIVE corpus, no common word may be discriminative. It catches stoplist under-reach
// that a small hand-picked negative set hides (the failure a reviewer found: "build",
// "data", "function", "value" etc. leaking unlisted). A failure names the leaking word
// and its validated df — add it to commonWords and re-run.
func TestPushGateExcludesCommonVocabulary(t *testing.T) {
	ctx := context.Background()
	root := os.Getenv("TWICESHY_LIVE_CORPUS")
	if root == "" {
		t.Skip("set TWICESHY_LIVE_CORPUS to a twiceshy-corpus checkout (or run make test-livecorpus)")
	}
	if _, err := os.Stat(filepath.Join(root, "experience")); err != nil {
		t.Fatalf("TWICESHY_LIVE_CORPUS=%q has no experience directory: %v", root, err)
	}
	recs, err := record.LoadCorpus(root)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) == 0 {
		t.Fatalf("TWICESHY_LIVE_CORPUS=%q loaded zero records", root)
	}
	ix, err := Open(filepath.Join(t.TempDir(), "vocab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(ctx, recs, ""); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, w := range strings.Fields(adversarialVocab) {
		if seen[w] {
			continue
		}
		seen[w] = true
		disc, _, err := ix.discriminativeTokens(ctx, w)
		if err != nil {
			t.Fatal(err)
		}
		if len(disc) > 0 {
			df, _ := ix.validatedDF(ctx, w)
			t.Errorf("common word %q is discriminative (validated df=%d) — add it to commonWords", w, df)
		}
	}
}
