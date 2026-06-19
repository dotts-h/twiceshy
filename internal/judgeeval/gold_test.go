// SPDX-License-Identifier: AGPL-3.0-only

package judgeeval_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/judgeeval"
)

// The embedded gold set is the eval's ground truth; if it is internally
// inconsistent the whole eval is meaningless, so CI guards it.
func TestLoadGold_ParsesAndIsConsistent(t *testing.T) {
	cases, err := judgeeval.LoadGold()
	if err != nil {
		t.Fatalf("LoadGold: %v", err)
	}
	if len(cases) < 20 {
		t.Fatalf("gold set unexpectedly small: %d cases", len(cases))
	}

	byMode := map[string]int{}
	ids := map[string]bool{}
	for _, c := range cases {
		if ids[c.ID] {
			t.Errorf("duplicate case id %q", c.ID)
		}
		ids[c.ID] = true
		byMode[c.Mode]++

		// Each case must render a judging request the judge can actually read.
		req := c.Request()
		if req.Record == nil || strings.TrimSpace(req.Record.Title) == "" {
			t.Errorf("%s: empty record/title", c.ID)
		}
		if len(req.Repros) == 0 {
			t.Errorf("%s: no repros", c.ID)
		}

		// Ground-truth consistency: approve ⇔ no failing checks; reject ⇔ ≥1.
		switch c.WantDecision {
		case judge.Approve:
			if len(c.WantFailingChecks) != 0 {
				t.Errorf("%s: approve case lists failing checks %v", c.ID, c.WantFailingChecks)
			}
			if c.Mode != "approve" {
				t.Errorf("%s: approve decision but mode %q", c.ID, c.Mode)
			}
		case judge.Reject:
			if len(c.WantFailingChecks) == 0 {
				t.Errorf("%s: reject case names no failing check", c.ID)
			}
		default:
			t.Errorf("%s: bad want_decision %q", c.ID, c.WantDecision)
		}
	}

	// The set must span every failure mode with a meaningful number of cases.
	for _, m := range judgeeval.Modes {
		if byMode[m] < 3 {
			t.Errorf("mode %q has only %d cases; want a spread across every mode", m, byMode[m])
		}
	}
	if byMode["approve"] == 0 {
		t.Error("no clean-approve control cases — false-reject would be unmeasurable")
	}
}
