// SPDX-License-Identifier: AGPL-3.0-only

package guard_test

import (
	"testing"

	"github.com/dotts-h/twiceshy/internal/guard"
)

func TestEngaged_EmergencyStop(t *testing.T) {
	if !(guard.Guardrails{Paused: true}).Engaged() {
		t.Fatal("a paused guardrail must be engaged")
	}
	if (guard.Guardrails{}).Engaged() {
		t.Fatal("the default guardrail must not be engaged")
	}
}

func TestBudget_UnlimitedByDefault(t *testing.T) {
	b := guard.Guardrails{}.Budget() // MaxRuns 0 = unlimited
	for i := 0; i < 1000; i++ {
		if !b.AllowRun() {
			t.Fatalf("unlimited budget refused run %d", i)
		}
		b.StartRun()
	}
	if b.Anomalous() {
		t.Fatal("MaxActions 0 must never be anomalous")
	}
}

func TestBudget_RunCapStops(t *testing.T) {
	b := guard.Guardrails{MaxRuns: 3}.Budget()
	allowed := 0
	for i := 0; i < 10; i++ {
		if !b.AllowRun() {
			break
		}
		b.StartRun()
		allowed++
	}
	if allowed != 3 {
		t.Fatalf("processed %d runs, want the cap of 3", allowed)
	}
	if b.Runs() != 3 {
		t.Fatalf("Runs() = %d, want 3", b.Runs())
	}
}

func TestBudget_PromotionCapStops(t *testing.T) {
	// MaxPromotions is the intended throughput cap: a clean stop at the ceiling,
	// distinct from the anomaly halt. It bounds promotions (CountAction), not the
	// broker/judge runs (StartRun) that MaxRuns bounds.
	b := guard.Guardrails{MaxPromotions: 3}.Budget()
	taken := 0
	for i := 0; i < 10; i++ {
		if b.Capped() {
			break
		}
		b.CountAction()
		taken++
	}
	if taken != 3 {
		t.Fatalf("took %d promotions, want the cap of 3", taken)
	}
	if !b.Capped() {
		t.Fatal("at the cap, Capped() must be true")
	}
}

func TestBudget_PromotionCapUnlimitedByDefault(t *testing.T) {
	b := guard.Guardrails{}.Budget() // MaxPromotions 0 = unlimited
	for i := 0; i < 1000; i++ {
		if b.Capped() {
			t.Fatalf("unlimited cap stopped at promotion %d", i)
		}
		b.CountAction()
	}
}

// A throughput cap below the anomaly threshold stops the run CLEANLY before the
// anomaly halt can trip — so a normal full batch is never mis-flagged as a
// compromised-judge spike (the bug: MaxActions doubling as the throttle).
func TestBudget_CapStopsBeforeAnomaly(t *testing.T) {
	b := guard.Guardrails{MaxPromotions: 3, MaxActions: 25}.Budget()
	for !b.Capped() {
		b.CountAction()
	}
	if b.Anomalous() {
		t.Fatal("a run stopped at the throughput cap must not be anomalous")
	}
}

// Footgun guard: when a throughput cap is set, the count-anomaly is the cap's
// concern, not a separate halt — even if actions somehow exceed MaxActions the
// run is NOT flagged anomalous (the cap is the governor; that's what stops it).
func TestBudget_CapDisablesCountAnomaly(t *testing.T) {
	b := guard.Guardrails{MaxPromotions: 100, MaxActions: 25}.Budget()
	for i := 0; i < 30; i++ { // 30 > MaxActions 25, but below the cap
		b.CountAction()
	}
	if b.Anomalous() {
		t.Fatal("with a throughput cap set, the raw count-anomaly must not fire")
	}
}

// Unbounded mode (no cap): the count-anomaly remains the compromised-judge backstop.
func TestBudget_AnomalyBackstopWhenUncapped(t *testing.T) {
	b := guard.Guardrails{MaxActions: 25}.Budget() // MaxPromotions 0 = unbounded
	for i := 0; i < 26; i++ {
		b.CountAction()
	}
	if !b.Anomalous() {
		t.Fatal("uncapped, 26 actions over threshold 25 must be anomalous")
	}
}

func TestBudget_AnomalyThreshold(t *testing.T) {
	b := guard.Guardrails{MaxActions: 2}.Budget()
	b.CountAction()
	if b.Anomalous() {
		t.Fatal("1 action under threshold 2 must not be anomalous")
	}
	b.CountAction()
	if b.Anomalous() {
		t.Fatal("2 actions at threshold 2 must not be anomalous (>, not >=)")
	}
	b.CountAction()
	if !b.Anomalous() {
		t.Fatal("3 actions over threshold 2 must be anomalous")
	}
	if b.Actions() != 3 {
		t.Fatalf("Actions() = %d, want 3", b.Actions())
	}
}

// #0085: an approval-RATE anomaly that survives a throughput cap. Under a cap the
// count-anomaly is moot (Anomalous() returns false), but a compromised judge
// approving ~everything still shows up as a high promoted/judged fraction.
// RateAnomalous() flags a run whose action rate exceeds MaxActionRate, gated on a
// minimum sample so a tiny run isn't flagged.
func TestBudget_RateAnomalyFiresUnderCap(t *testing.T) {
	// Capped run, judge approves everything: 10 judged, 10 promoted = 100% > 60%
	// baseline, sample 10 >= MinSample 5. The count-anomaly is moot under the cap,
	// yet the rate-anomaly fires — the gap #0085 closes.
	b := guard.Guardrails{MaxPromotions: 10, MaxActions: 25, MaxActionRate: 0.6, MinSample: 5}.Budget()
	for i := 0; i < 10; i++ {
		b.StartRun()
		b.CountAction()
	}
	if b.Anomalous() {
		t.Fatal("the count-anomaly must stay moot under a throughput cap")
	}
	if !b.RateAnomalous() {
		t.Fatalf("100%% approval over the baseline must be a rate anomaly; rate=%v", b.ActionRate())
	}
}

// A normal mixed run (most records held) has a low action rate and does NOT trip.
func TestBudget_RateAnomalyQuietOnNormalRun(t *testing.T) {
	// 50 judged, 10 promoted = 20% < 60% baseline.
	b := guard.Guardrails{MaxPromotions: 10, MaxActionRate: 0.6, MinSample: 5}.Budget()
	for i := 0; i < 50; i++ {
		b.StartRun()
	}
	for i := 0; i < 10; i++ {
		b.CountAction()
	}
	if b.RateAnomalous() {
		t.Fatalf("a 20%% approval rate must not be anomalous; rate=%v", b.ActionRate())
	}
}

// Minimum sample: a tiny run (3/3 = 100%) is NOT flagged — too little signal, exactly
// the case #0085 calls out ("a tiny run of 3/3 isn't flagged").
func TestBudget_RateAnomalyNeedsMinSample(t *testing.T) {
	b := guard.Guardrails{MaxActionRate: 0.6, MinSample: 5}.Budget()
	for i := 0; i < 3; i++ {
		b.StartRun()
		b.CountAction()
	}
	if b.RateAnomalous() {
		t.Fatal("a 3/3 run below the minimum sample must not be flagged")
	}
	if b.ActionRate() != 1.0 {
		t.Fatalf("ActionRate() = %v, want 1.0", b.ActionRate())
	}
}

// Disabled by default: MaxActionRate 0 never fires, regardless of rate/sample — the
// rollout pattern (off until an operator opts in), like the throughput cap.
func TestBudget_RateAnomalyDisabledByDefault(t *testing.T) {
	b := guard.Guardrails{}.Budget() // MaxActionRate 0 = off
	for i := 0; i < 100; i++ {
		b.StartRun()
		b.CountAction() // 100% rate, large sample
	}
	if b.RateAnomalous() {
		t.Fatal("MaxActionRate 0 must disable the rate anomaly")
	}
}

// Threshold is strict (>, not >=): a rate exactly at the baseline does not trip.
func TestBudget_RateAnomalyStrictThreshold(t *testing.T) {
	b := guard.Guardrails{MaxActionRate: 0.5, MinSample: 5}.Budget()
	for i := 0; i < 10; i++ {
		b.StartRun()
	}
	for i := 0; i < 5; i++ {
		b.CountAction() // 5/10 = 0.5 exactly, at the baseline
	}
	if b.RateAnomalous() {
		t.Fatalf("a rate exactly at the baseline must not trip (>, not >=); rate=%v", b.ActionRate())
	}
	b.CountAction() // 6/10 = 0.6 > 0.5
	if !b.RateAnomalous() {
		t.Fatalf("6/10 over the 0.5 baseline must trip; rate=%v", b.ActionRate())
	}
}

// ActionRate is promoted/judged, 0 when nothing was judged (no divide-by-zero).
func TestBudget_ActionRateZeroWhenNothingJudged(t *testing.T) {
	b := guard.Guardrails{MaxActionRate: 0.6, MinSample: 0}.Budget()
	if b.ActionRate() != 0 {
		t.Fatalf("ActionRate() with no runs = %v, want 0", b.ActionRate())
	}
	if b.RateAnomalous() {
		t.Fatal("no judged records must not be a rate anomaly even with MinSample 0")
	}
}

func TestTruthy(t *testing.T) {
	for _, s := range []string{"1", "true", "TRUE", "yes", "on", " on "} {
		if !guard.Truthy(s) {
			t.Errorf("Truthy(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "0", "false", "no", "off", "x"} {
		if guard.Truthy(s) {
			t.Errorf("Truthy(%q) = true, want false", s)
		}
	}
}
