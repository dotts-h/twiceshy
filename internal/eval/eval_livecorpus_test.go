//go:build livecorpus

// SPDX-License-Identifier: AGPL-3.0-only

package eval_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/eval"
	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// TestPushPrecisionOnLiveCorpus is the push gate's regression guard, run against
// the REAL corpus (../..): off-domain prompts must inject NOTHING (precision) and
// genuine traps must still surface (recall). It guards the exact failure the spike
// found — the discriminative gate leaking common dev vocabulary as the corpus grew.
func TestPushPrecisionOnLiveCorpus(t *testing.T) {
	ctx := context.Background()
	recs, err := record.LoadCorpus("../..")
	if err != nil {
		t.Skipf("live corpus unavailable at ../.. (decoupled to twiceshy-corpus, ADR-0021): %v", err)
	}
	ix, err := index.Open(filepath.Join(t.TempDir(), "push.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(ctx, recs, ""); err != nil {
		t.Fatal(err)
	}

	cases := append(eval.PushNegatives(), eval.PushPositives()...)
	rep, err := eval.RunPush(ctx, ix, cases)
	if err != nil {
		t.Fatalf("RunPush: %v", err)
	}

	// Precision: zero off-domain injection is the whole point of push.
	if rep.FalseInjections != 0 {
		for _, l := range rep.Leaks {
			t.Errorf("push leaked on off-domain prompt: %s", l)
		}
		t.Fatalf("push precision = %.2f (%d/%d off-domain prompts injected); want 1.00",
			rep.Precision(), rep.FalseInjections, rep.Negatives)
	}
	// Recall: tightening the gate must not silence the genuine traps.
	if rep.Recalled != rep.Positives {
		for _, m := range rep.Misses {
			t.Errorf("push dropped a genuine trap: %s", m)
		}
		t.Fatalf("push recall = %.2f (%d/%d traps surfaced); want 1.00",
			rep.Recall(), rep.Recalled, rep.Positives)
	}
}
