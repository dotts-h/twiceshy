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
