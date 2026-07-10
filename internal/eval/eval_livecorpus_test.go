//go:build livecorpus

// SPDX-License-Identifier: AGPL-3.0-only

package eval_test

import (
	"context"
	"os"
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
