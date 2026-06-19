// SPDX-License-Identifier: AGPL-3.0-only

package judgeeval

import (
	"context"
	"errors"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/record"
)

// gcase builds a minimal scoring fixture; the stub keys off record.ID.
func gcase(id, mode string, dec judge.Decision, checks ...judge.CheckName) Case {
	return Case{
		ID: id, Mode: mode, WantDecision: dec, WantFailingChecks: checks, Rationale: "x",
		record: &record.Record{ID: id, Title: "title"},
		repros: []judge.ReproArtifact{{Path: "p", Kind: "positive"}},
	}
}

func vApprove() judge.Verdict { return judge.ApproveVerdict("stub") }

func vReject(fail ...judge.CheckName) judge.Verdict {
	failset := make(map[judge.CheckName]bool, len(fail))
	for _, f := range fail {
		failset[f] = true
	}
	checks := make([]judge.Check, len(judge.Checks))
	for i, n := range judge.Checks {
		checks[i] = judge.Check{Name: n, Pass: !failset[n], Reason: "r"}
	}
	dec := judge.Approve
	if len(fail) > 0 {
		dec = judge.Reject
	}
	return judge.Verdict{Decision: dec, Checks: checks, Model: "stub"}
}

// fnJudge is a per-case stub: fn maps (record id, call index) to a verdict/error.
type fnJudge struct {
	fn    func(id string, call int) (judge.Verdict, error)
	calls map[string]int
}

func (f *fnJudge) Judge(_ context.Context, req judge.Request) (judge.Verdict, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	id := req.Record.ID
	n := f.calls[id]
	f.calls[id]++
	return f.fn(id, n)
}

func TestRun_PerfectJudge(t *testing.T) {
	cases := []Case{
		gcase("A1", "approve", judge.Approve),
		gcase("P1", "poison", judge.Reject, judge.Poison),
		gcase("S1", "scope", judge.Reject, judge.Scope),
	}
	// A perfect judge approves the approve case and rejects the reject cases
	// failing exactly the expected check.
	stub := &fnJudge{fn: func(id string, _ int) (judge.Verdict, error) {
		switch id {
		case "A1":
			return vApprove(), nil
		case "P1":
			return vReject(judge.Poison), nil
		default:
			return vReject(judge.Scope), nil
		}
	}}
	res, err := Run(context.Background(), stub, cases, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.FalseApproves != 0 || res.FalseRejects != 0 || res.Errors != 0 {
		t.Fatalf("perfect judge: %+v", res)
	}
	if res.Accuracy != 1.0 {
		t.Errorf("accuracy = %v, want 1.0", res.Accuracy)
	}
	if res.FalseApproveRate != 0 {
		t.Errorf("false-approve rate = %v, want 0", res.FalseApproveRate)
	}
	if res.CheckRecall != 1.0 {
		t.Errorf("check recall = %v, want 1.0 (both rejected for the right reason)", res.CheckRecall)
	}
}

func TestRun_FalseApproveIsFailUnsafe(t *testing.T) {
	cases := []Case{
		gcase("P1", "poison", judge.Reject, judge.Poison),
		gcase("P2", "poison", judge.Reject, judge.Poison),
	}
	// The judge wrongly APPROVES P1 (a record that should be rejected).
	stub := &fnJudge{fn: func(id string, _ int) (judge.Verdict, error) {
		if id == "P1" {
			return vApprove(), nil
		}
		return vReject(judge.Poison), nil
	}}
	res, _ := Run(context.Background(), stub, cases, 1)
	if res.FalseApproves != 1 {
		t.Fatalf("FalseApproves = %d, want 1", res.FalseApproves)
	}
	if res.FalseApproveRate != 0.5 {
		t.Errorf("FalseApproveRate = %v, want 0.5 (1 of 2 reject cases)", res.FalseApproveRate)
	}
	// The false-approve must be flagged on the right outcome.
	for _, o := range res.Outcomes {
		if o.CaseID == "P1" && !o.FalseApprove {
			t.Errorf("P1 should be a false-approve")
		}
	}
}

func TestRun_FalseReject(t *testing.T) {
	cases := []Case{gcase("A1", "approve", judge.Approve)}
	stub := &fnJudge{fn: func(string, int) (judge.Verdict, error) { return vReject(judge.Meaning), nil }}
	res, _ := Run(context.Background(), stub, cases, 1)
	if res.FalseRejects != 1 || res.FalseRejectRate != 1.0 {
		t.Fatalf("want 1 false-reject over 1 approve case, got %+v", res)
	}
	if res.FalseApproves != 0 {
		t.Errorf("a false-reject must not be counted as a false-approve")
	}
}

func TestRun_ErrorIsNeitherApproveNorReject(t *testing.T) {
	// A judge outage (error) on a reject case must NOT count as a false-approve
	// (fail-safe: no verdict → stays quarantined), but it is not "correct" either.
	cases := []Case{gcase("P1", "poison", judge.Reject, judge.Poison)}
	stub := &fnJudge{fn: func(string, int) (judge.Verdict, error) { return judge.Verdict{}, errors.New("boom") }}
	res, _ := Run(context.Background(), stub, cases, 1)
	if res.Errors != 1 {
		t.Fatalf("Errors = %d, want 1", res.Errors)
	}
	if res.FalseApproves != 0 {
		t.Errorf("an errored reject case must never be a false-approve")
	}
	if res.Correct != 0 {
		t.Errorf("an errored case is not correct")
	}
}

func TestRun_MajorityVote(t *testing.T) {
	// repeat=3: approve,approve,reject → majority approve. For a reject case that
	// makes it a false-approve.
	cases := []Case{gcase("P1", "poison", judge.Reject, judge.Poison)}
	seq := []judge.Verdict{vApprove(), vApprove(), vReject(judge.Poison)}
	stub := &fnJudge{fn: func(_ string, call int) (judge.Verdict, error) { return seq[call], nil }}
	res, _ := Run(context.Background(), stub, cases, 3)
	if res.FalseApproves != 1 {
		t.Fatalf("majority approve should yield a false-approve, got %+v", res.Outcomes)
	}
}

func TestRun_CheckRecallRewardsRightReason(t *testing.T) {
	cases := []Case{
		gcase("S1", "scope", judge.Reject, judge.Scope),     // rejected for the right reason
		gcase("M1", "meaning", judge.Reject, judge.Meaning), // rejected, but for the WRONG check
	}
	stub := &fnJudge{fn: func(id string, _ int) (judge.Verdict, error) {
		if id == "S1" {
			return vReject(judge.Scope), nil
		}
		return vReject(judge.Poison), nil // rejects M1 but blames poison, not meaning
	}}
	res, _ := Run(context.Background(), stub, cases, 1)
	if res.FalseApproves != 0 || res.Correct != 2 {
		t.Fatalf("both should be correctly rejected, got %+v", res)
	}
	// Both rejected; only S1 caught an expected check → recall 1/2.
	if res.CheckRecall != 0.5 {
		t.Errorf("CheckRecall = %v, want 0.5", res.CheckRecall)
	}
}

func TestRun_DualDefectCaseCountsEitherCheck(t *testing.T) {
	// S3-style: ground truth allows scope OR meaning. Failing meaning counts.
	cases := []Case{gcase("S3", "scope", judge.Reject, judge.Scope, judge.Meaning)}
	stub := &fnJudge{fn: func(string, int) (judge.Verdict, error) { return vReject(judge.Meaning), nil }}
	res, _ := Run(context.Background(), stub, cases, 1)
	if res.CheckRecall != 1.0 {
		t.Errorf("a dual-defect case caught on either listed check should score recall 1.0, got %v", res.CheckRecall)
	}
}

func TestRun_ByModeBreakdown(t *testing.T) {
	cases := []Case{
		gcase("A1", "approve", judge.Approve),
		gcase("P1", "poison", judge.Reject, judge.Poison),
	}
	stub := &fnJudge{fn: func(id string, _ int) (judge.Verdict, error) {
		if id == "A1" {
			return vApprove(), nil
		}
		return vReject(judge.Poison), nil
	}}
	res, _ := Run(context.Background(), stub, cases, 1)
	stats := res.ByMode()
	if len(stats) != 2 {
		t.Fatalf("want 2 mode stats, got %d", len(stats))
	}
	if stats[0].Mode != "approve" {
		t.Errorf("modes should be in canonical order, got %q first", stats[0].Mode)
	}
}
