//go:build livecorpus

// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dotts-h/twiceshy/internal/eval"
	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// TestRetrievePushPrecisionRecall is the push-channel relevance guard. It runs
// against the live corpus (record.LoadCorpus) so a future record that closes the
// document-frequency gap fails here rather than silently re-noising push. It
// shares eval's canonical cases instead of maintaining a second set that can rot
// into self-referential false positives as the corpus records those old probes.
func TestRetrievePushPrecisionRecall(t *testing.T) {
	ix := openIndex(t, liveCorpusRecords(t))
	ctx := context.Background()

	for _, c := range eval.PushNegatives() {
		hits, err := ix.RetrievePush(ctx, index.Query{Text: c.Query})
		if err != nil {
			t.Fatalf("RetrievePush(%q): %v", c.Query, err)
		}
		if len(hits) != 0 {
			t.Errorf("off-topic %q injected %d card(s) %v, want 0", c.Query, len(hits), hitIDs(hits))
		}
	}

	for _, c := range eval.PushPositives() {
		hits, err := ix.RetrievePush(ctx, index.Query{Text: c.Query})
		if err != nil {
			t.Fatalf("RetrievePush(%q): %v", c.Query, err)
		}
		got := hitIDs(hits)
		if !hasID(got, c.ExpectID) {
			t.Errorf("on-topic %q -> %v, want %s present", c.Query, got, c.ExpectID)
		}
	}
}

// TestRetrievePushTraced exposes the gate decision for telemetry (#0067) without
// changing what RetrievePush serves: the discriminative tokens that opened the
// gate and the served hits. Off-topic queries leave the decision empty.
// Moved to livecorpus because the push gate floor is corpus-scale-dependent.
func TestRetrievePushTraced(t *testing.T) {
	ix := openIndex(t, liveCorpusRecords(t))
	ctx := context.Background()

	t.Run("discriminative query records its gate tokens + served hits", func(t *testing.T) {
		const q = "fts5 bm25 scores are negative so order by rank desc returns the worst rows"
		d, err := ix.RetrievePushTraced(ctx, index.Query{Text: q})
		if err != nil {
			t.Fatal(err)
		}
		if d.FingerprintBypass {
			t.Error("a non-signature query must not record a fingerprint bypass")
		}
		if !slices.Contains(d.Discriminative, "bm25") {
			t.Errorf("gate tokens should include the discriminative term: %v", d.Discriminative)
		}
		if !hasID(hitIDs(d.Served), "exp-0002") {
			t.Errorf("served should include exp-0002: %v", hitIDs(d.Served))
		}
		// The trace must serve EXACTLY what RetrievePush serves — no behavioral drift.
		plain, err := ix.RetrievePush(ctx, index.Query{Text: q})
		if err != nil {
			t.Fatal(err)
		}
		if len(plain) != len(d.Served) {
			t.Errorf("wrapper drift: RetrievePush served %d, traced served %d", len(plain), len(d.Served))
		}
	})

	t.Run("off-topic query records an empty decision", func(t *testing.T) {
		d, err := ix.RetrievePushTraced(ctx, index.Query{Text: "what is a good birthday gift to buy for my mother this year"})
		if err != nil {
			t.Fatal(err)
		}
		if d.FingerprintBypass || len(d.Discriminative) != 0 || len(d.Served) != 0 {
			t.Errorf("off-topic must be an empty decision: %+v", d)
		}
	})
}

func liveCorpusRecords(t *testing.T) []*record.Record {
	t.Helper()
	root := os.Getenv("TWICESHY_LIVE_CORPUS")
	if root == "" {
		t.Skip("set TWICESHY_LIVE_CORPUS to a twiceshy-corpus checkout (or run make test-livecorpus)")
	}
	if _, err := os.Stat(filepath.Join(root, "experience")); err != nil {
		t.Fatalf("TWICESHY_LIVE_CORPUS=%q has no experience directory: %v", root, err)
	}
	recs, err := record.LoadCorpus(root)
	if err != nil {
		t.Fatalf("LoadCorpus(%q): %v", root, err)
	}
	if len(recs) == 0 {
		t.Fatalf("TWICESHY_LIVE_CORPUS=%q loaded zero records", root)
	}
	return recs
}

func hitIDs(hits []index.Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.ID
	}
	return out
}

func hasID(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}
