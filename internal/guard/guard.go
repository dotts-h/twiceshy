// SPDX-License-Identifier: AGPL-3.0-only

// Package guard holds the safety net ADR-0013 §7 makes part of the closed-loop
// decision (#0033): the limits the autonomous promote (#0029) and demote (#0032)
// loops consult so the residual risks the gate + judge can't cover — chiefly an
// available-but-compromised judge ("who judges the judge") and a report_outcome
// DoS — are bounded:
//
//   - emergency stop  — one switch halts ALL auto-promotion/demotion; records
//     pile up quarantined/disputed (fail-safe), nothing auto-releases.
//   - anomaly monitor — a promotion/demotion rate past a threshold raises a
//     notification (a judge that suddenly approves everything is caught, not
//     discovered in production). It alerts; the pause is what halts.
//   - budget cap      — a ceiling on the broker/judge runs one invocation can
//     trigger, so a report flood can't drain the sandbox.
package guard

import "strings"

// Guardrails are the configured safety limits for one promote/adapt invocation.
type Guardrails struct {
	// Paused is the emergency stop: when true, nothing is auto-promoted or
	// auto-demoted (fail-safe).
	Paused bool
	// MaxActions is the anomaly alert threshold — promotions/demotions per run
	// above which a notification fires. 0 disables the alert.
	MaxActions int
	// MaxPromotions is the intended throughput cap — the per-run ceiling on
	// promotions/demotions at which the loop stops CLEANLY (a normal, mergeable
	// batch; "re-run to continue"). It is distinct from MaxActions: set below
	// MaxActions it bounds a normal batch, so a full run is never mis-flagged as a
	// compromised-judge spike. 0 is unlimited (then MaxActions is the only ceiling).
	MaxPromotions int
	// MaxActionRate is the approval-RATE anomaly baseline (#0085): the fraction of
	// judged records promoted/demoted above which a run is flagged as a likely
	// compromised judge. Unlike MaxActions this is NOT moot under a throughput cap —
	// a capped run that approves ~everything still shows a high promoted/judged
	// fraction — so it is the spike detector that survives capped (production) mode.
	// 0 disables it (off until an operator opts in, like MaxPromotions).
	MaxActionRate float64
	// MinSample is the minimum number of judged records (StartRun calls) before the
	// rate anomaly can fire, so a tiny run (e.g. 3/3) is not flagged on too little
	// signal. Below it, RateAnomalous() always returns false.
	MinSample int
	// MaxRuns is the budget cap — records processed (broker/judge runs) per
	// invocation. 0 is unlimited.
	MaxRuns int
}

// Engaged reports whether the emergency stop is on.
func (g Guardrails) Engaged() bool { return g.Paused }

// Budget returns a fresh per-invocation counter for these guardrails.
func (g Guardrails) Budget() *Budget { return &Budget{g: g} }

// Budget tracks one invocation's broker/judge runs and actions against the
// guardrails. Single-goroutine — the promote/adapt loops are sequential.
type Budget struct {
	g       Guardrails
	runs    int
	actions int
}

// AllowRun reports whether another record may be processed — i.e. the broker/
// judge budget is not yet exhausted.
func (b *Budget) AllowRun() bool { return b.g.MaxRuns == 0 || b.runs < b.g.MaxRuns }

// StartRun records that a record is about to be processed (one broker/judge run).
func (b *Budget) StartRun() { b.runs++ }

// Runs returns how many records have been processed.
func (b *Budget) Runs() int { return b.runs }

// CountAction records a promotion or demotion.
func (b *Budget) CountAction() { b.actions++ }

// Actions returns how many promotions/demotions have been taken.
func (b *Budget) Actions() int { return b.actions }

// Capped reports whether the intended throughput cap is reached — a clean stop
// at the per-run ceiling, NOT an anomaly. 0 (unset) never caps.
func (b *Budget) Capped() bool { return b.g.MaxPromotions > 0 && b.actions >= b.g.MaxPromotions }

// Anomalous reports whether the raw action count has crossed the alert threshold
// — the blunt "judge approving everything" backstop for UNBOUNDED runs. When a
// throughput cap is set (MaxPromotions > 0) the cap is the governor: a normal run
// stops cleanly at the cap, so the count-anomaly is moot and reports false (it
// would otherwise mis-flag every capped batch). In capped mode the raw-count
// backstop is gone; RateAnomalous (#0085) is the approval-RATE anomaly that survives
// a cap and catches a compromised judge there.
func (b *Budget) Anomalous() bool {
	if b.g.MaxPromotions > 0 {
		return false
	}
	return b.g.MaxActions > 0 && b.actions > b.g.MaxActions
}

// ActionRate is the fraction of judged records that resulted in a promotion/demotion
// (actions/runs) for this run — 0 when nothing has been judged (no divide-by-zero).
func (b *Budget) ActionRate() float64 {
	if b.runs == 0 {
		return 0
	}
	return float64(b.actions) / float64(b.runs)
}

// RateAnomalous reports whether this run's action rate exceeds MaxActionRate over at
// least MinSample judged records — the compromised-judge backstop that SURVIVES a
// throughput cap (#0085). Where Anomalous() (a raw count) goes moot once
// MaxPromotions caps the run, a judge approving ~everything still shows as a high
// promoted/judged fraction here. Disabled when MaxActionRate <= 0; quiet below
// MinSample (too little signal) and on an empty run.
func (b *Budget) RateAnomalous() bool {
	if b.g.MaxActionRate <= 0 || b.runs == 0 || b.runs < b.g.MinSample {
		return false
	}
	return b.ActionRate() > b.g.MaxActionRate
}

// Truthy parses an on/off environment flag (e.g. TWICESHY_PAUSE).
func Truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
