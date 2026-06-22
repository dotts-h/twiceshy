// SPDX-License-Identifier: AGPL-3.0-only

package judgeeval_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/judgeeval"
	"github.com/dotts-h/twiceshy/internal/record"
)

// #0063: an advisory-class gold case (vuln id, no repro) must (a) load — the loader
// exempts advisory records from the repro requirement — and (b) render via the
// advisory prompt, i.e. Case.Request().Advisory mirrors record.IsAdvisoryClass for
// every case. Scoring an advisory under the prose rubric measures the wrong thing.
func TestLoadGold_AdvisoryCasesRouteToAdvisoryPrompt(t *testing.T) {
	cases, err := judgeeval.LoadGold()
	if err != nil {
		t.Fatalf("LoadGold: %v", err)
	}
	advisoryCases := 0
	for _, c := range cases {
		req := c.Request()
		want := record.IsAdvisoryClass(req.Record)
		if req.Advisory != want {
			t.Errorf("case %s: Request().Advisory=%v, want %v (must mirror IsAdvisoryClass)", c.ID, req.Advisory, want)
		}
		if want {
			advisoryCases++
			if len(req.Repros) != 0 {
				t.Errorf("case %s: advisory case must carry no repro, got %d", c.ID, len(req.Repros))
			}
		}
	}
	if advisoryCases == 0 {
		t.Fatal("gold set has no advisory-class case — the #0063 routing is unguarded")
	}
}

// #0074: the 85 Sonnet advisory verdicts load as advisory-class gold cases (66 approve
// / 19 reject), each routed to the advisory prompt with no repro. Their gold-case ids
// are the corpus record ids (exp-NNNN); the prose set uses A1/P1/... so the prefix
// isolates the generated advisory set. Guards advisory-gold.yaml against silent
// loss or regeneration drift.
func TestLoadGold_IncludesSonnetAdvisorySet(t *testing.T) {
	cases, err := judgeeval.LoadGold()
	if err != nil {
		t.Fatalf("LoadGold: %v", err)
	}
	approve, reject := 0, 0
	for _, c := range cases {
		if !strings.HasPrefix(c.ID, "exp-") {
			continue
		}
		req := c.Request()
		if !req.Advisory {
			t.Errorf("advisory gold case %s must route to the advisory prompt", c.ID)
		}
		if len(req.Repros) != 0 {
			t.Errorf("advisory gold case %s must carry no repro, got %d", c.ID, len(req.Repros))
		}
		switch c.WantDecision {
		case judge.Approve:
			approve++
		case judge.Reject:
			reject++
		}
	}
	if approve != 66 || reject != 19 {
		t.Errorf("advisory set: got %d approve / %d reject, want 66 / 19 (the Sonnet audit)", approve, reject)
	}
}

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
		// Advisory-class cases (ADR-0016) carry no repro by design (#0063); every
		// other case must render a repro the judge can read.
		if len(req.Repros) == 0 && !req.Advisory {
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
