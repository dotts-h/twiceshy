// SPDX-License-Identifier: AGPL-3.0-only

package promote

import (
	"context"
	"fmt"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// DisputeThreshold is the number of independent corroborating outcome reports
// about one record at which a non-reproducible failure flips it to `disputed`
// (escalate to a human). It is > 1 by design so one — or a few — confused or
// hostile reports can never stealth-neuter a good card: only execution-backed
// counter-evidence (a reproduced failure, judge-gated) demotes (ADR-0013 §3).
const DisputeThreshold = 3

// Action is what the counter-evidence gate decided for one report.
type Action string

const (
	// ActionNone leaves the record unchanged (not reproduced and uncorroborated,
	// judge declined, or inconclusive — a fail-safe no-op).
	ActionNone Action = "none"
	// ActionDemote flips a record validated→stale: reproduced failure + judge PASS.
	ActionDemote Action = "demote"
	// ActionDispute flips a record validated→disputed: independent non-reproducing
	// reports corroborated past the threshold (escalate; reversible).
	ActionDispute Action = "dispute"
)

// CounterEvidence is the broker re-run the gate adjudicates: the original
// record's repro re-run, and the outcome-report's evidence run as a counter-repro
// (CounterRepro is that materialized script, handed to the judge as the proof).
// The CLI runs the broker and fills this in; the Adapter only decides.
type CounterEvidence struct {
	Original     repro.Attestation
	Counter      repro.Attestation
	CounterRepro string
}

// AdaptOutcome is the gate's decision for one report.
type AdaptOutcome struct {
	Action  Action
	Reason  string
	Verdict judge.Verdict
}

// Adapter is the negative direction of ADR-0013's closed loop (#0032): an
// outcome report plus the broker re-run plus the diverse judge decide whether to
// demote, dispute, or hold a record. It mirrors Promoter and reuses the same
// judge seam. Like the positive direction it is fail-safe: a misapplied or
// hostile report can only ever *propose* work, and only execution-backed
// counter-evidence (judge-gated) can demote a card.
type Adapter struct {
	judge judge.Judge
	now   func() string // demotion valid.until date, "YYYY-MM-DD"
}

// AdaptOption configures an Adapter.
type AdaptOption func(*Adapter)

// WithAdaptClock injects the clock (tests pin it).
func WithAdaptClock(now func() string) AdaptOption { return func(a *Adapter) { a.now = now } }

// NewAdapter builds an Adapter around the diverse judge.
func NewAdapter(j judge.Judge, opts ...AdaptOption) *Adapter {
	a := &Adapter{judge: j, now: todayUTC}
	for _, o := range opts {
		o(a)
	}
	return a
}

// Adapt adjudicates one outcome report against the record it disputes, given the
// broker re-run (ev) and the count of OTHER reports disputing the same record
// (corroborating). It mutates original in place ONLY when it demotes or disputes;
// otherwise original is untouched. A judge outage is fail-safe (ActionNone, nil),
// distinct from a caller bug (mismatched report/original → error).
func (a *Adapter) Adapt(ctx context.Context, original, report *record.Record, ev CounterEvidence, corroborating int) (AdaptOutcome, error) {
	if report.Provenance.Disputes == nil || *report.Provenance.Disputes != original.ID {
		return AdaptOutcome{}, fmt.Errorf("adapt: report %s does not dispute %s", report.ID, original.ID)
	}
	// Only a currently-served record can be demoted or disputed.
	if original.Status != "validated" {
		return none("original is not validated (nothing to demote)"), nil
	}

	origBroke := !ev.Original.Inconclusive && !ev.Original.Holds
	counterReproduced := !ev.Counter.Inconclusive && ev.Counter.Holds

	if origBroke || counterReproduced {
		// Execution-backed counter-evidence — the judge gates the demotion.
		verdict, err := a.judge.Judge(ctx, judge.Request{
			Record:      report,
			Attestation: ev.Counter,
			Repros:      []judge.ReproArtifact{{Path: "counter", Kind: "counter", Label: "outcome-report evidence", Content: ev.CounterRepro}},
		})
		if err != nil {
			return none("judge unavailable — no demotion (fail-safe): " + err.Error()), nil
		}
		if !verdict.Approved() {
			return AdaptOutcome{Action: ActionNone, Reason: "judge did not approve the demotion", Verdict: verdict}, nil
		}
		if err := a.demote(original, report, ev, verdict); err != nil {
			return AdaptOutcome{}, err
		}
		return AdaptOutcome{Action: ActionDemote, Verdict: verdict}, nil
	}

	// Not reproduced. A counter that could not even run is no signal at all — it
	// must never count toward disputing a card (the attribution guard).
	if ev.Counter.Inconclusive {
		return none("counter-repro inconclusive (could not run) — no signal"), nil
	}
	// The counter ran and did not reproduce. Only corroboration past the threshold
	// (this report + the others) escalates the card to disputed — never one report.
	if corroborating+1 >= DisputeThreshold {
		if err := a.dispute(original); err != nil {
			return AdaptOutcome{}, err
		}
		return AdaptOutcome{Action: ActionDispute, Reason: "non-reproducing reports corroborated past threshold — escalated"}, nil
	}
	return none("not reproduced; below the corroboration threshold — accumulate"), nil
}

func none(reason string) AdaptOutcome { return AdaptOutcome{Action: ActionNone, Reason: reason} }

// demote flips original validated→stale with the audit trail, reverting on any
// invalidity so the caller never persists a half-demoted record.
func (a *Adapter) demote(original, report *record.Record, ev CounterEvidence, verdict judge.Verdict) error {
	origStatus, origUntil, origDem := original.Status, original.Provenance.Valid.Until, original.Provenance.Demotion
	now := a.now()
	attestedAt := ev.Counter.RanAt
	if attestedAt == "" {
		attestedAt = ev.Original.RanAt
	}
	if attestedAt == "" {
		attestedAt = now // defensive: the broker always stamps ran_at, but never emit an empty audit
	}
	original.Status = "stale"
	original.Provenance.Valid.Until = &now
	original.Provenance.Demotion = &record.Demotion{
		AttestedAt:    attestedAt,
		JudgeModel:    verdict.Model,
		JudgeDecision: string(verdict.Decision),
		Report:        report.ID,
	}
	if err := record.Validate(original); err != nil {
		original.Status, original.Provenance.Valid.Until, original.Provenance.Demotion = origStatus, origUntil, origDem
		return fmt.Errorf("adapt: demoted record %s is invalid (not persisted): %w", original.ID, err)
	}
	return nil
}

// dispute flags original validated→disputed (reversible; escalated to a human),
// reverting on any invalidity.
func (a *Adapter) dispute(original *record.Record) error {
	origStatus := original.Status
	original.Status = "disputed"
	if err := record.Validate(original); err != nil {
		original.Status = origStatus
		return fmt.Errorf("adapt: disputed record %s is invalid (not persisted): %w", original.ID, err)
	}
	return nil
}
