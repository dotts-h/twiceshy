// SPDX-License-Identifier: AGPL-3.0-only

package eval

import (
	"context"
	"testing"

	"github.com/dotts-h/twiceshy/internal/retro"
)

// RunUsage micro-averages the usage judge's accuracy over SERVED cards only — a verdict
// for a never-served id must be ignored, the same trust boundary the live join enforces.
// This locks the TP/FP/FN counting and the precision/recall math (the part that is easy to
// get subtly wrong); the live judge's real accuracy is measured by running the command.
func TestRunUsage_PrecisionRecallMath(t *testing.T) {
	cases := []UsageCase{{
		Name:       "controlled",
		Transcript: "irrelevant — the stub ignores the transcript",
		Served:     []string{"exp-0001", "exp-0002", "exp-0003"},
		Used:       []string{"exp-0001", "exp-0002"}, // gold
	}}
	// The stub ignores the transcript and returns fixed verdicts:
	//   exp-0001 used  -> TP (served + gold-used)
	//   exp-0002 not   -> FN (gold-used, judge missed it)
	//   exp-0003 used  -> FP (served, not gold-used)
	//   exp-0099 used  -> ignored entirely (never served)
	judge := &retro.StubUsageJudge{Verdicts: []retro.CardVerdict{
		{ID: "exp-0001", Used: true},
		{ID: "exp-0002", Used: false},
		{ID: "exp-0003", Used: true},
		{ID: "exp-0099", Used: true},
	}}

	rep, err := RunUsage(context.Background(), judge, cases)
	if err != nil {
		t.Fatalf("RunUsage: %v", err)
	}
	if rep.TP != 1 || rep.FP != 1 || rep.FN != 1 {
		t.Fatalf("TP/FP/FN = %d/%d/%d, want 1/1/1 (the unserved exp-0099 must be ignored)", rep.TP, rep.FP, rep.FN)
	}
	if p := rep.Precision(); p != 0.5 {
		t.Errorf("precision = %v, want 0.5 (1 TP of 2 judge-positives)", p)
	}
	if r := rep.Recall(); r != 0.5 {
		t.Errorf("recall = %v, want 0.5 (1 TP of 2 gold-used)", r)
	}
}

// A zero report is precision/recall 1.0 (no false positives / nothing missed), never a
// divide-by-zero — mirrors PushReport's empty-set convention.
func TestUsageReport_EmptyIsOne(t *testing.T) {
	var r UsageReport
	if r.Precision() != 1 || r.Recall() != 1 {
		t.Errorf("empty report precision/recall = %v/%v, want 1/1", r.Precision(), r.Recall())
	}
}
