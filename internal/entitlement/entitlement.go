// SPDX-License-Identifier: AGPL-3.0-only

// Package entitlement defines feature-gated commercial plan capabilities. It
// deliberately contains no billing state or provider integration (ADR-0035).
package entitlement

import "fmt"

type Plan string

const (
	Community  Plan = "community"
	Pro        Plan = "pro"
	Team       Plan = "team"
	Enterprise Plan = "enterprise"
)

type QuotaPolicy struct {
	DailyCalls    int
	RatePerMinute int
}

type Entitlements struct {
	Plan  Plan
	Quota QuotaPolicy
}

var catalog = map[Plan]Entitlements{
	Community: {Plan: Community, Quota: QuotaPolicy{DailyCalls: 1000, RatePerMinute: 60}},
	Pro:       {Plan: Pro, Quota: QuotaPolicy{DailyCalls: 5000, RatePerMinute: 300}},
	Team:      {Plan: Team, Quota: QuotaPolicy{DailyCalls: 20000, RatePerMinute: 600}},
	// Zero means "server default 60", not unlimited, at tenantAuth. Keep the
	// enterprise rate explicit and bounded so it cannot fall below Pro/Team.
	Enterprise: {Plan: Enterprise, Quota: QuotaPolicy{DailyCalls: 0, RatePerMinute: 6000}},
}

func ParsePlan(raw string) (Plan, error) {
	plan := Plan(raw)
	if _, ok := catalog[plan]; !ok {
		return "", fmt.Errorf("unknown plan %q (want community, pro, team, or enterprise)", raw)
	}
	return plan, nil
}

func ForPlan(plan Plan) (Entitlements, error) {
	entitlements, ok := catalog[plan]
	if !ok {
		return Entitlements{}, fmt.Errorf("unknown plan %q", plan)
	}
	return entitlements, nil
}
