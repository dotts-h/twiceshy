// SPDX-License-Identifier: AGPL-3.0-only

package judge_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
)

// sequenceJudge returns the primed verdicts/errors in order, one per call, and
// counts calls — so a test can drive a flapping judge across a vote.
type sequenceJudge struct {
	verdicts []judge.Verdict
	errs     []error
	calls    int
}

func (s *sequenceJudge) Judge(context.Context, judge.Request) (judge.Verdict, error) {
	i := s.calls
	s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		return judge.Verdict{}, s.errs[i]
	}
	return s.verdicts[i], nil
}

func approve() judge.Verdict { return judge.ApproveVerdict("gemini-2.5-pro") }
func reject() judge.Verdict  { return judge.Verdict{Decision: judge.Reject} }

// rejectFrom is a rejecting verdict that names its model, so a test can prove which
// rejecting member the representative-reject selection returned.
func rejectFrom(model string) judge.Verdict {
	return judge.Verdict{Decision: judge.Reject, Model: model}
}

func TestMajority_StrictMajorityApproves(t *testing.T) {
	// approve, approve, reject → 2/3 → majority approve.
	sj := &sequenceJudge{verdicts: []judge.Verdict{approve(), approve(), reject()}}
	v, err := judge.NewMajority(sj, 3).Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if sj.calls != 3 {
		t.Fatalf("want 3 votes, got %d", sj.calls)
	}
	if !v.Approved() {
		t.Fatalf("a 2/3 majority must return an approving verdict: %+v", v)
	}
	// The returned verdict must be a REAL member's approving verdict, not a synthesized
	// one — the promotion audit records its Model (promote.go) and Checks. A regression
	// that returned a bare Approve while staying Approved() would lose the audit detail.
	if v.Model != "gemini-2.5-pro" {
		t.Fatalf("representative approving verdict must preserve the member's Model, got %q", v.Model)
	}
	if len(v.Checks) != len(judge.Checks) {
		t.Fatalf("representative verdict must carry the member's Checks, got %d want %d", len(v.Checks), len(judge.Checks))
	}
}

// All votes reject (approvals==0): the representative-reject selection must return the
// LAST-seen rejecting verdict (carrying its Model for the audit), not the zero Verdict.
func TestMajority_AllRejectReturnsRepresentativeReject(t *testing.T) {
	sj := &sequenceJudge{verdicts: []judge.Verdict{rejectFrom("m1"), rejectFrom("m2"), rejectFrom("m3")}}
	v, err := judge.NewMajority(sj, 3).Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if v.Approved() {
		t.Fatalf("an all-reject vote must NOT approve: %+v", v)
	}
	if v.Model != "m3" {
		t.Fatalf("representative-reject must be the last-seen rejecting verdict (m3), not the zero Verdict; got Model=%q", v.Model)
	}
}

func TestMajority_MinorityStaysQuarantined(t *testing.T) {
	// approve, reject, reject → 1/3 → NOT a majority; must return non-approving.
	sj := &sequenceJudge{verdicts: []judge.Verdict{approve(), reject(), reject()}}
	v, err := judge.NewMajority(sj, 3).Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if v.Approved() {
		t.Fatalf("a 1/3 minority must NOT approve: %+v", v)
	}
}

// Even votes: a tie is not a strict majority (fail-safe).
func TestMajority_TieDoesNotApprove(t *testing.T) {
	sj := &sequenceJudge{verdicts: []judge.Verdict{approve(), reject()}}
	v, err := judge.NewMajority(sj, 2).Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if v.Approved() {
		t.Fatalf("a 1/1 tie must NOT approve: %+v", v)
	}
}

// Any inner error aborts the vote and propagates (the gate fails safe).
func TestMajority_ErrorMidVoteAborts(t *testing.T) {
	sj := &sequenceJudge{
		verdicts: []judge.Verdict{approve(), {}, {}},
		errs:     []error{nil, errors.New("endpoint down"), nil},
	}
	_, err := judge.NewMajority(sj, 3).Judge(context.Background(), judge.Request{})
	if err == nil {
		t.Fatal("an inner judge error must propagate (fail-safe)")
	}
	if sj.calls != 2 {
		t.Fatalf("the vote must abort on the first error, after 2 calls; got %d", sj.calls)
	}
}

// votes < 1 clamps to a single call (no voting), preserving the single-shot path.
func TestMajority_ClampsToAtLeastOne(t *testing.T) {
	sj := &sequenceJudge{verdicts: []judge.Verdict{approve()}}
	v, err := judge.NewMajority(sj, 0).Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if sj.calls != 1 || !v.Approved() {
		t.Fatalf("votes<1 must clamp to 1 approving call; calls=%d approved=%v", sj.calls, v.Approved())
	}
}
