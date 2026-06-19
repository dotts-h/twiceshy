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
