// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

func TestPromoteCorpus_EmergencyStopHalts(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true, "exp-0101": true}}
	var persisted []string
	persist := func(_ string, r *record.Record) error { persisted = append(persisted, r.ID); return nil }
	var buf bytes.Buffer

	st, _, err := promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{Paused: true}, nil, nil, &buf, "")
	if err != nil {
		t.Fatalf("promoteCorpus: %v", err)
	}
	if st.promoted != 0 || len(persisted) != 0 || len(fp.calls) != 0 {
		t.Fatalf("emergency stop must halt all promotions; promoted=%d persisted=%v calls=%v", st.promoted, persisted, fp.calls)
	}
	if !strings.Contains(buf.String(), "emergency stop") {
		t.Fatalf("expected an emergency-stop notice; got %q", buf.String())
	}
}

func TestPromoteCorpus_BudgetCapStops(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101"), eligibleRec("exp-0102")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true, "exp-0101": true, "exp-0102": true}}
	persist := func(_ string, _ *record.Record) error { return nil }
	var buf bytes.Buffer

	st, _, err := promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{MaxRuns: 1}, nil, nil, &buf, "")
	if err != nil {
		t.Fatalf("promoteCorpus: %v", err)
	}
	if len(fp.calls) != 1 || st.promoted != 1 {
		t.Fatalf("budget cap of 1 run must process one record; calls=%v promoted=%d", fp.calls, st.promoted)
	}
	if !strings.Contains(buf.String(), "budget cap") {
		t.Fatalf("expected a budget-cap notice; got %q", buf.String())
	}
}

func TestPromoteCorpus_ThroughputCapStopsCleanly(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101"), eligibleRec("exp-0102")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true, "exp-0101": true, "exp-0102": true}}
	persist := func(_ string, _ *record.Record) error { return nil }
	var buf bytes.Buffer

	// MaxPromotions 1 with MaxActions 25: a normal full batch must stop CLEANLY at
	// the cap (err == nil, NOT errAnomalyHalt) so the batch PR is mergeable — the
	// #0084 fix for "every batch trips the anomaly halt" (cap doubling as throttle).
	st, _, err := promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{MaxPromotions: 1, MaxActions: 25}, nil, nil, &buf, "")
	if err != nil {
		t.Fatalf("a throughput-cap stop must be clean (nil), got %v", err)
	}
	if st.promoted != 1 {
		t.Fatalf("throughput cap of 1 must promote exactly one; promoted=%d", st.promoted)
	}
	if !strings.Contains(buf.String(), "throughput cap") {
		t.Fatalf("expected a throughput-cap notice; got %q", buf.String())
	}
	if strings.Contains(buf.String(), "ANOMALY") {
		t.Fatalf("a clean cap stop must not mention ANOMALY; got %q", buf.String())
	}
}

func TestPromoteCorpus_AnomalyOnFinalActionStillHalts(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true, "exp-0101": true}}
	persist := func(_ string, _ *record.Record) error { return nil }
	var buf bytes.Buffer

	// MaxActions 1, exactly 2 records: the 2nd promotion trips the threshold but
	// there is no further record to halt before. The run must STILL report the
	// anomaly (errAnomalyHalt → non-zero exit) so a spike on the last action can't
	// slip through with exit 0 (#0037, ADR-0013 §D1).
	st, _, err := promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{MaxActions: 1}, nil, nil, &buf, "")
	if !errors.Is(err, errAnomalyHalt) {
		t.Fatalf("a threshold-tripping run must return errAnomalyHalt, got %v", err)
	}
	if st.promoted != 2 {
		t.Fatalf("both promotions persist (nothing after to halt); promoted=%d, want 2", st.promoted)
	}
}

func TestAdaptCorpus_EmergencyStopHalts(t *testing.T) {
	orig := validatedRec("exp-0043")
	rep := reportRec("exp-0200", "exp-0043")
	recs := []*record.Record{orig, rep}
	runner := fakeCounterRunner{ev: map[string]promote.CounterEvidence{
		"exp-0043": {Original: repro.Attestation{Holds: true}, Counter: repro.Attestation{Holds: true}, CounterRepro: "x"},
	}}
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")})
	var persisted []string
	persist := func(_ string, r *record.Record) error { persisted = append(persisted, r.ID); return nil }
	var buf bytes.Buffer

	st, _, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{Paused: true}, nil, nil, &buf, "")
	if err != nil {
		t.Fatalf("adaptCorpus: %v", err)
	}
	if st.demoted != 0 || len(persisted) != 0 {
		t.Fatalf("emergency stop must halt all demotions; demoted=%d persisted=%v", st.demoted, persisted)
	}
	if orig.Status != "validated" {
		t.Fatalf("the disputed record must be untouched under the emergency stop; status=%q", orig.Status)
	}
	if !strings.Contains(buf.String(), "emergency stop") {
		t.Fatalf("expected an emergency-stop notice; got %q", buf.String())
	}
}

// adaptDemoting builds N reports each disputing its own validated original, with
// a counter-runner that makes every counter reproduce (so each demotes).
func adaptDemoting(t *testing.T, n int) ([]*record.Record, fakeCounterRunner) {
	t.Helper()
	var recs []*record.Record
	ev := map[string]promote.CounterEvidence{}
	for i := 0; i < n; i++ {
		oid := "exp-010" + string(rune('0'+i))
		recs = append(recs, validatedRec(oid), reportRec("exp-020"+string(rune('0'+i)), oid))
		ev[oid] = promote.CounterEvidence{Original: repro.Attestation{Holds: true}, Counter: repro.Attestation{Holds: true}, CounterRepro: "x"}
	}
	return recs, fakeCounterRunner{ev: ev}
}

func TestAdaptCorpus_BudgetCapStops(t *testing.T) {
	recs, runner := adaptDemoting(t, 3)
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("g")})
	persist := func(_ string, _ *record.Record) error { return nil }
	var buf bytes.Buffer

	st, _, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{MaxRuns: 1}, nil, nil, &buf, "")
	if err != nil {
		t.Fatalf("adaptCorpus: %v", err)
	}
	if st.demoted != 1 {
		t.Fatalf("budget cap of 1 run must demote one; demoted=%d", st.demoted)
	}
	if !strings.Contains(buf.String(), "budget cap") {
		t.Fatalf("expected a budget-cap notice; got %q", buf.String())
	}
}

func TestAdaptCorpus_ThroughputCapStopsCleanly(t *testing.T) {
	recs, runner := adaptDemoting(t, 3)
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("g")})
	persist := func(_ string, _ *record.Record) error { return nil }
	var buf bytes.Buffer

	// MaxPromotions 1 with MaxActions 25: the adapt batch stops CLEANLY at the cap
	// (err == nil, not errAnomalyHalt) — symmetric with promote (#0084).
	st, _, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{MaxPromotions: 1, MaxActions: 25}, nil, nil, &buf, "")
	if err != nil {
		t.Fatalf("a throughput-cap stop must be clean (nil), got %v", err)
	}
	if st.demoted != 1 {
		t.Fatalf("throughput cap of 1 must demote exactly one; demoted=%d", st.demoted)
	}
	if !strings.Contains(buf.String(), "throughput cap") {
		t.Fatalf("expected a throughput-cap notice; got %q", buf.String())
	}
}

func TestAdaptCorpus_AnomalyOnFinalActionStillHalts(t *testing.T) {
	recs, runner := adaptDemoting(t, 2)
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("g")})
	persist := func(_ string, _ *record.Record) error { return nil }
	var buf bytes.Buffer

	// MaxActions 1, 2 demotes: the 2nd trips the threshold with nothing after to
	// halt — the run must still return errAnomalyHalt (non-zero exit).
	st, _, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{MaxActions: 1}, nil, nil, &buf, "")
	if !errors.Is(err, errAnomalyHalt) {
		t.Fatalf("a threshold-tripping adapt run must return errAnomalyHalt, got %v", err)
	}
	if st.demoted != 2 {
		t.Fatalf("both demotions persist (nothing after to halt); demoted=%d, want 2", st.demoted)
	}
}

// #0085: the approval-RATE anomaly survives a throughput cap. Five eligible records,
// judge approves ALL, cap 3: the count-anomaly is moot under the cap, but 3/3 promoted
// = 100% over the 50% baseline (min sample 3) trips the rate anomaly post-loop — the
// run halts (errAnomalyHalt) even though the cap was the clean stop.
func TestPromoteCorpus_RateAnomalyHaltsUnderCap(t *testing.T) {
	recs := []*record.Record{
		eligibleRec("exp-0100"), eligibleRec("exp-0101"), eligibleRec("exp-0102"),
		eligibleRec("exp-0103"), eligibleRec("exp-0104"),
	}
	fp := &fakePromoter{promote: map[string]bool{
		"exp-0100": true, "exp-0101": true, "exp-0102": true, "exp-0103": true, "exp-0104": true,
	}}
	persist := func(_ string, _ *record.Record) error { return nil }
	var buf bytes.Buffer

	st, _, err := promoteCorpus(context.Background(), ".", recs, fp, persist,
		guard.Guardrails{MaxPromotions: 3, MaxActionRate: 0.5, MinSample: 3}, nil, nil, &buf, "")
	if !errors.Is(err, errAnomalyHalt) {
		t.Fatalf("a high approval rate under a cap must return errAnomalyHalt, got %v", err)
	}
	if st.promoted != 3 {
		t.Fatalf("the cap promotes 3 before the post-loop rate halt; promoted=%d", st.promoted)
	}
	// Assert the full numeric body, not just the headline: this pins the denominator
	// (3/3, the rate is promoted/JUDGED), the rendered percentage, the baseline, and
	// the min-sample — so a denominator regression or a message-template drift is caught.
	if !strings.Contains(buf.String(), "3/3 promoted (100%) over the 50% baseline (min sample 3)") {
		t.Fatalf("expected the full approval-rate anomaly body; got %q", buf.String())
	}
}

// Inverse of the above (#0085): a capped run with a HIGH rate but FEWER judged records
// than MinSample must NOT halt — too little signal — even with the rate knob enabled.
// Pins that the cmd wiring respects the MinSample gate inside RateAnomalous (a raw
// ActionRate()>baseline check, dropping the gate, would wrongly halt this run).
func TestPromoteCorpus_RateAnomalyQuietBelowMinSample(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true, "exp-0101": true}}
	persist := func(_ string, _ *record.Record) error { return nil }
	var buf bytes.Buffer

	// 2 judged, both promoted = 100% rate, but MinSample 5 > 2 → no halt, clean stop.
	st, _, err := promoteCorpus(context.Background(), ".", recs, fp, persist,
		guard.Guardrails{MaxPromotions: 5, MaxActionRate: 0.5, MinSample: 5}, nil, nil, &buf, "")
	if err != nil {
		t.Fatalf("a run below the minimum sample must be a clean stop, got %v", err)
	}
	if st.promoted != 2 {
		t.Fatalf("both eligible records promote; promoted=%d", st.promoted)
	}
	if strings.Contains(buf.String(), "ANOMALY") {
		t.Fatalf("a run below the minimum sample must not flag an anomaly; got %q", buf.String())
	}
}

// Symmetric with promote (#0085): 5 reports each demoting its original, cap 3, judge
// demotes ALL → 3/3 = 100% over the 50% baseline (min sample 3). The rate anomaly
// halts the adapt run even under the cap.
func TestAdaptCorpus_RateAnomalyHaltsUnderCap(t *testing.T) {
	recs, runner := adaptDemoting(t, 5)
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("g")})
	persist := func(_ string, _ *record.Record) error { return nil }
	var buf bytes.Buffer

	st, _, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist,
		guard.Guardrails{MaxPromotions: 3, MaxActionRate: 0.5, MinSample: 3}, nil, nil, &buf, "")
	if !errors.Is(err, errAnomalyHalt) {
		t.Fatalf("a high demote rate under a cap must return errAnomalyHalt, got %v", err)
	}
	if st.demoted != 3 {
		t.Fatalf("the cap demotes 3 before the post-loop rate halt; demoted=%d", st.demoted)
	}
	// Full numeric body — pins the denominator (3/3 = demote-or-dispute/JUDGED), the
	// percentage, baseline, and min-sample, not just the headline.
	if !strings.Contains(buf.String(), "3/3 demote/dispute actions (100%) over the 50% baseline (min sample 3)") {
		t.Fatalf("expected the full action-rate anomaly body; got %q", buf.String())
	}
}
