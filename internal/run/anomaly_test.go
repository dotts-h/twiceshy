// SPDX-License-Identifier: AGPL-3.0-only

package run

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// A promotion spike past the alert threshold means a compromised judge approving
// everything. The run must HALT before persisting further (not persist-then-
// check-then-continue-then-exit-0) and return a distinct anomaly signal (#0037,
// ADR-0013 §D1).
func TestPromoteCorpus_AnomalyHaltsBeforeFurtherWrites(t *testing.T) {
	recs := []*record.Record{
		eligibleRec("exp-0100"), eligibleRec("exp-0101"),
		eligibleRec("exp-0102"), eligibleRec("exp-0103"),
	}
	fp := &fakePromoter{promote: map[string]bool{
		"exp-0100": true, "exp-0101": true, "exp-0102": true, "exp-0103": true,
	}}
	var persisted []string
	persist := func(_ string, rec *record.Record) error { persisted = append(persisted, rec.ID); return nil }

	// MaxActions=1: count 1 (ok), 2 (>1 → anomalous); the check runs BEFORE the
	// next persist, so the 3rd promotion is halted, the 4th never reached.
	st, _, err := PromoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{MaxActions: 1}, nil, nil, &bytes.Buffer{}, "")
	if !errors.Is(err, ErrAnomalyHalt) {
		t.Fatalf("an anomalous run must return ErrAnomalyHalt, got %v", err)
	}
	if len(persisted) != 2 {
		t.Fatalf("anomaly must stop further writes: persisted %v, want exactly the 2 before the trip", persisted)
	}
	if st.Promoted != 2 {
		t.Fatalf("promoted = %d, want 2 (halted before the rest)", st.Promoted)
	}
}

// A clean run with the anomaly monitor off (MaxActions=0) never halts.
func TestPromoteCorpus_NoAnomalyWhenMonitorOff(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101"), eligibleRec("exp-0102")}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true, "exp-0101": true, "exp-0102": true}}
	var persisted []string
	persist := func(_ string, rec *record.Record) error { persisted = append(persisted, rec.ID); return nil }

	_, _, err := PromoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{MaxActions: 0}, nil, nil, &bytes.Buffer{}, "")
	if err != nil {
		t.Fatalf("monitor off must never halt: %v", err)
	}
	if len(persisted) != 3 {
		t.Fatalf("all 3 must persist, got %v", persisted)
	}
}

func TestAdaptCorpus_AnomalyHaltsBeforeFurtherWrites(t *testing.T) {
	o1, o2, o3 := validatedRec("exp-0041"), validatedRec("exp-0042"), validatedRec("exp-0043")
	r1 := reportRec("exp-0201", "exp-0041")
	r2 := reportRec("exp-0202", "exp-0042")
	r3 := reportRec("exp-0203", "exp-0043")
	recs := []*record.Record{o1, o2, o3, r1, r2, r3}
	demote := promote.CounterEvidence{Original: repro.Attestation{Holds: true}, Counter: repro.Attestation{Holds: true}, CounterRepro: "x"}
	runner := fakeCounterRunner{ev: map[string]promote.CounterEvidence{
		"exp-0041": demote, "exp-0042": demote, "exp-0043": demote,
	}}
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")})
	var persisted []string
	persist := func(_ string, r *record.Record) error { persisted = append(persisted, r.ID); return nil }

	st, _, err := AdaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{MaxActions: 1}, nil, nil, &bytes.Buffer{}, "")
	if !errors.Is(err, ErrAnomalyHalt) {
		t.Fatalf("an anomalous adapt run must return ErrAnomalyHalt, got %v", err)
	}
	if len(persisted) != 2 {
		t.Fatalf("anomaly must stop further demotes: persisted %v, want exactly 2", persisted)
	}
	if st.Demoted != 2 {
		t.Fatalf("demoted = %d, want 2", st.Demoted)
	}
}

// exitCode maps the anomaly halt to a process code distinct from a usage error
// or a generic failure, so an unattended wrapper can react to it specifically.
