// SPDX-License-Identifier: AGPL-3.0-only

package entitlement_test

import (
	"testing"

	"github.com/dotts-h/twiceshy/internal/entitlement"
)

func TestPlansHaveDerivedQuotaPolicies(t *testing.T) {
	want := map[entitlement.Plan]entitlement.QuotaPolicy{
		entitlement.Community:  {DailyCalls: 1000, RatePerMinute: 60},
		entitlement.Pro:        {DailyCalls: 5000, RatePerMinute: 300},
		entitlement.Team:       {DailyCalls: 20000, RatePerMinute: 600},
		entitlement.Enterprise: {DailyCalls: 0, RatePerMinute: 6000},
	}
	for plan, policy := range want {
		got, err := entitlement.ForPlan(plan)
		if err != nil {
			t.Fatalf("ForPlan(%q): %v", plan, err)
		}
		if got.Plan != plan || got.Quota != policy {
			t.Errorf("ForPlan(%q) = %+v, want quota %+v", plan, got, policy)
		}
	}
}

func TestParsePlanRejectsUnknown(t *testing.T) {
	if _, err := entitlement.ParsePlan("starter-plus"); err == nil {
		t.Fatal("unknown plan must be rejected")
	}
}
