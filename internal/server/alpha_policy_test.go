// SPDX-License-Identifier: AGPL-3.0-only

package server

import "testing"

// alphaWriteSurfaces is the declared list of every write tool that must carry
// a contribution quota under ADR-0031's single policy seam. Any NEW write
// tool MUST be added here AND to alphaContributionQuotas in alpha_policy.go —
// this test is the guard that makes "a write tool skipped the policy" a build
// failure, not a review catch (#0136).
var alphaWriteSurfaces = []string{"record_experience", "report_outcome", "report_issue", "confirm_helpful"}

// TestAlphaContributionQuotasDeclareEveryWriteSurface is ADR-0031's
// completeness test: every declared write surface has a POSITIVE quota in
// alphaContributionQuotas (a non-positive quota fails closed elsewhere, but
// must never be the declared value), and the map holds no unknown/renamed
// keys either — bidirectional, so a rename on one side without the other
// fails too.
func TestAlphaContributionQuotasDeclareEveryWriteSurface(t *testing.T) {
	for _, tool := range alphaWriteSurfaces {
		limit, ok := alphaContributionQuotas[tool]
		if !ok {
			t.Errorf("alphaContributionQuotas is missing %q — every write surface must declare a quota (ADR-0031)", tool)
			continue
		}
		if limit <= 0 {
			t.Errorf("alphaContributionQuotas[%q] = %d, want > 0 (non-positive means unbounded — never allowed for an alpha write)", tool, limit)
		}
	}

	declared := make(map[string]bool, len(alphaWriteSurfaces))
	for _, tool := range alphaWriteSurfaces {
		declared[tool] = true
	}
	for tool := range alphaContributionQuotas {
		if !declared[tool] {
			t.Errorf("alphaContributionQuotas has unknown key %q not in alphaWriteSurfaces — add it there too, or it is a stale rename", tool)
		}
	}
}
