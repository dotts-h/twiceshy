// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// fakePromoter is a stub recordPromoter: it promotes the ids in `promote`,
// errors for ids in `err`, and otherwise holds. On a promotion it mutates the
// record like the real promoter so persist sees the flip.
type fakePromoter struct {
	promote map[string]bool
	err     map[string]error
	calls   []string
}

func (f *fakePromoter) Promote(_ context.Context, rec *record.Record) (promote.Outcome, error) {
	f.calls = append(f.calls, rec.ID)
	if e, ok := f.err[rec.ID]; ok {
		return promote.Outcome{}, e
	}
	if f.promote[rec.ID] {
		rec.Status = "validated"
		return promote.Outcome{
			Promoted:    true,
			Verdict:     judge.Verdict{Model: "gemini-2.5-pro", Decision: judge.Approve},
			Attestation: repro.Attestation{ReproducedUnder: []string{"go1.25"}},
		}, nil
	}
	return promote.Outcome{Reason: "judge did not approve"}, nil
}

func eligibleRec(id string) *record.Record {
	rp := "experience/repro/" + id + ".sh"
	return &record.Record{
		ID: id, Status: "quarantined",
		Guard: &record.Guard{Repro: &rp},
		// Resolution with a substantive root_cause is required by the
		// promote root-cause pre-gate (#0094): records without one are held.
		Resolution: &record.Resolution{RootCause: "stub root cause for corpus-level orchestration test"},
		Path:       "experience/2026/" + id[len("exp-"):] + "-x.md",
	}
}

func TestRunPromote_RequiresJudgeURL(t *testing.T) {
	var buf bytes.Buffer
	err := runPromote(context.Background(), []string{"-corpus", corpus}, &buf,
		func(string) string { return "" }) // no TWICESHY_JUDGE_URL
	if err == nil || !strings.Contains(err.Error(), "TWICESHY_JUDGE_URL") {
		t.Fatalf("auto-promotion without a judge must fail safe; got %v", err)
	}
}

func TestRunPromote_RejectsLocalJudgeModel(t *testing.T) {
	var buf bytes.Buffer
	err := runPromote(context.Background(), []string{"-corpus", corpus, "-judge-model", "llama3.2"}, &buf,
		func(k string) string {
			if k == "TWICESHY_JUDGE_URL" {
				return "http://judge.local"
			}
			return ""
		})
	if err == nil || !strings.Contains(err.Error(), "judge") {
		t.Fatalf("the cheap local model must be rejected as judge; got %v", err)
	}
}

func TestRunPromote_DryRunWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	err := runPromote(context.Background(), []string{"-corpus", corpus, "-dry-run"}, &buf,
		func(string) string { return "" })
	if err != nil {
		t.Fatalf("dry-run must not need a judge: %v", err)
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Fatalf("dry-run output missing; got %q", buf.String())
	}
}
