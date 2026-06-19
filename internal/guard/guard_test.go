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
