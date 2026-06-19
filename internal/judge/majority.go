// SPDX-License-Identifier: AGPL-3.0-only

package judge

import "context"

// DefaultVotes is the production repeat-N (#0041, ADR-0013 §F1): gpt-oss:20b is
// non-deterministic at temp 0 on boundary cases (~0.7% single-shot false-approve,
// exp-0046), so production judges each record an odd number of times and takes
// the majority. 3 was the smallest N that closed the measured gap; 5 measured 0%.
const DefaultVotes = 3

// MajorityJudge wraps a Judge and decides by majority of N independent calls,
// turning the single-shot non-determinism into a stable verdict. It is fail-safe
// in both directions:
//   - any inner error aborts the vote and propagates (the caller treats a judge
//     error as not-approved, ADR-0013 §6);
//   - a non-strict-majority of approvals returns a NON-approving verdict, so the
//     gate keeps the record quarantined.
type MajorityJudge struct {
	inner Judge
	votes int
}

// NewMajority wraps inner to judge by majority of votes calls. votes < 1 is
// clamped to 1 (a single call — no voting). It returns the Judge interface so it
// drops into any judge seam (the promote gate).
func NewMajority(inner Judge, votes int) Judge {
	if votes < 1 {
		votes = 1
	}
	return MajorityJudge{inner: inner, votes: votes}
}

// Judge calls the inner judge `votes` times and returns a representative
// approving verdict iff a strict majority approved; otherwise a representative
// non-approving verdict. A strict majority is approvals*2 > votes, so a tie (only
// possible at even votes) does NOT approve — fail-safe.
func (m MajorityJudge) Judge(ctx context.Context, req Request) (Verdict, error) {
	approvals := 0
	var approving, rejecting Verdict
	sawRejecting := false
	for i := 0; i < m.votes; i++ {
		v, err := m.inner.Judge(ctx, req)
		if err != nil {
			return Verdict{}, err
		}
		if v.Approved() {
			approvals++
			approving = v
		} else {
			rejecting = v
			sawRejecting = true
		}
	}
	if approvals*2 > m.votes {
		return approving, nil
	}
	if sawRejecting {
		return rejecting, nil
	}
	// Unreachable when votes >= 1 (a minority implies at least one non-approval),
	// but the zero Verdict is not Approved(), so it still fails safe.
	return Verdict{}, nil
}
