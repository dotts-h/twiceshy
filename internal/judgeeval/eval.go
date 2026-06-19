// SPDX-License-Identifier: AGPL-3.0-only

package judgeeval

import (
	"context"
	"sort"

	"github.com/dotts-h/twiceshy/internal/judge"
)

// Run scores a judge against the gold set. caller is any judge.Judge (the real
// judge.ModelJudge for a live A/B, or a stub in tests). repeat samples each case
// repeat times and takes the majority decision — at temperature 0 the judge is
// near-deterministic, but a repeat>1 smooths the boundary cases and exposes
// non-determinism. The scoring is the point: FalseApproveRate is the fail-UNSAFE
// metric (a record that should be rejected but got approved would auto-promote),
// and it is the headline the winner is chosen on.
func Run(ctx context.Context, caller judge.Judge, cases []Case, repeat int) (Result, error) {
	if repeat < 1 {
		repeat = 1
	}
	res := Result{Repeat: repeat, Cases: len(cases)}
	for _, c := range cases {
		o := scoreCase(ctx, caller, c, repeat)
		if c.ShouldReject() {
			res.RejectCases++
		} else {
			res.ApproveCases++
		}
		if o.Errored {
			res.Errors++
		}
		if o.FalseApprove {
			res.FalseApproves++
		}
		if o.FalseReject {
			res.FalseRejects++
		}
		if o.Correct {
			res.Correct++
		}
		if c.ShouldReject() && o.Got == judge.Reject && o.CaughtCheck {
			res.ChecksCaught++
		}
		if o.Flipped {
			res.Flips++
		}
		res.Outcomes = append(res.Outcomes, o)
	}
	if res.RejectCases > 0 {
		res.FalseApproveRate = float64(res.FalseApproves) / float64(res.RejectCases)
	}
	if res.ApproveCases > 0 {
		res.FalseRejectRate = float64(res.FalseRejects) / float64(res.ApproveCases)
	}
	if res.Cases > 0 {
		res.Accuracy = float64(res.Correct) / float64(res.Cases)
	}
	rejectedRejects := res.RejectCases - res.FalseApproves - rejectErrors(res.Outcomes)
	if rejectedRejects > 0 {
		res.CheckRecall = float64(res.ChecksCaught) / float64(rejectedRejects)
	}
	return res, nil
}

// scoreCase runs one case repeat times, reduces to a single decision (majority;
// a tie or error-plurality is resolved on the side of caution below), and scores
// it against ground truth.
func scoreCase(ctx context.Context, caller judge.Judge, c Case, repeat int) Outcome {
	o := Outcome{CaseID: c.ID, Mode: c.Mode, Want: c.WantDecision, WantChecks: c.WantFailingChecks, Samples: repeat}
	var approvals, rejects, errs int
	var lastErr error
	var approveV, rejectV judge.Verdict
	for i := 0; i < repeat; i++ {
		v, err := caller.Judge(ctx, c.Request())
		if err != nil {
			errs++
			lastErr = err
			continue
		}
		if v.Approved() {
			approvals++
			approveV = v
		} else {
			rejects++
			rejectV = v
		}
	}
	o.Approvals, o.Rejects, o.ErrSamples = approvals, rejects, errs
	o.Flipped = approvals > 0 && rejects > 0 // disagreed with itself across samples

	// Reduce to a single decision. A clear majority of approve or reject wins;
	// otherwise (errors dominate, or an even split) we report the most honest
	// outcome: errors as an error, an approve/reject tie as approve (the
	// fail-unsafe side, so a tie can never hide a false-approve).
	switch {
	case approvals*2 > repeat:
		o.Got = judge.Approve
		o.Verdict = approveV
	case rejects*2 > repeat:
		o.Got = judge.Reject
		o.Verdict = rejectV
	case errs >= approvals && errs >= rejects:
		o.Errored = true
		if lastErr != nil {
			o.Err = lastErr.Error()
		}
	case approvals >= rejects:
		o.Got = judge.Approve
		o.Verdict = approveV
	default:
		o.Got = judge.Reject
		o.Verdict = rejectV
	}

	o.GotFailing = failingChecks(o.Verdict)
	o.CaughtCheck = caughtExpected(c.WantFailingChecks, o.GotFailing)

	switch {
	case o.Errored:
		// No verdict. Not "correct" either way; counted in Errors.
	case o.Got == c.WantDecision:
		o.Correct = true
	case c.ShouldReject() && o.Got == judge.Approve:
		o.FalseApprove = true
	default: // wanted approve, got reject
		o.FalseReject = true
	}
	return o
}

func failingChecks(v judge.Verdict) []judge.CheckName {
	var out []judge.CheckName
	for _, c := range v.Checks {
		if !c.Pass {
			out = append(out, c.Name)
		}
	}
	return out
}

// caughtExpected reports whether the judge failed at least one of the checks
// ground truth expects. A reject case may legitimately fail more than one check;
// catching any expected one means it rejected for a right reason.
func caughtExpected(want, got []judge.CheckName) bool {
	if len(want) == 0 {
		return false
	}
	set := make(map[judge.CheckName]bool, len(got))
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if set[w] {
			return true
		}
	}
	return false
}

func rejectErrors(os []Outcome) int {
	n := 0
	for _, o := range os {
		if o.Errored && o.Want == judge.Reject {
			n++
		}
	}
	return n
}

// Outcome is one case's scored result.
type Outcome struct {
	CaseID  string
	Mode    string
	Want    judge.Decision
	Got     judge.Decision // "" when Errored
	Errored bool
	Err     string

	FalseApprove bool // wanted reject, got approve — the fail-UNSAFE error
	FalseReject  bool // wanted approve, got reject — over-conservative
	Correct      bool

	WantChecks  []judge.CheckName
	GotFailing  []judge.CheckName
	CaughtCheck bool // for a rejected reject-case: failed ≥1 expected check
	Verdict     judge.Verdict

	// Sample-level detail exposes the judge's (non-)determinism: with repeat>1,
	// Flipped means the judge disagreed with itself across samples — the boundary
	// cases where a single run is unreliable.
	Samples    int
	Approvals  int
	Rejects    int
	ErrSamples int
	Flipped    bool
}

// Result aggregates a run. FalseApproveRate (over the reject cases) is the metric
// the winner is chosen on; the rest are diagnostics.
type Result struct {
	Repeat       int
	Cases        int
	RejectCases  int
	ApproveCases int

	Errors        int
	FalseApproves int
	FalseRejects  int
	Correct       int
	ChecksCaught  int
	Flips         int // cases the judge decided inconsistently across samples (repeat>1)

	FalseApproveRate float64 // FalseApproves / RejectCases — fail-unsafe
	FalseRejectRate  float64 // FalseRejects / ApproveCases — over-conservative
	Accuracy         float64 // Correct / Cases
	CheckRecall      float64 // rejected-for-the-right-reason, over correctly-rejected cases

	Outcomes []Outcome
}

// ByMode breaks the outcomes down per failure mode, for the report. The returned
// slice is sorted by the canonical mode order.
func (r Result) ByMode() []ModeStat {
	stats := make(map[string]*ModeStat)
	for _, o := range r.Outcomes {
		s := stats[o.Mode]
		if s == nil {
			s = &ModeStat{Mode: o.Mode}
			stats[o.Mode] = s
		}
		s.Cases++
		switch {
		case o.Errored:
			s.Errors++
		case o.Correct:
			s.Correct++
		case o.FalseApprove:
			s.FalseApproves++
		case o.FalseReject:
			s.FalseRejects++
		}
	}
	out := make([]ModeStat, 0, len(stats))
	for _, s := range stats {
		out = append(out, *s)
	}
	order := map[string]int{}
	for i, m := range Modes {
		order[m] = i
	}
	sort.Slice(out, func(i, j int) bool { return order[out[i].Mode] < order[out[j].Mode] })
	return out
}

// ModeStat is the per-failure-mode tally in a Result.
type ModeStat struct {
	Mode          string
	Cases         int
	Correct       int
	FalseApproves int
	FalseRejects  int
	Errors        int
}
