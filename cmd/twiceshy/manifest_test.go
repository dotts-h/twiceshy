// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// actionByID indexes the manifest actions a run returned.
func actionByID(actions []promote.RecordAction) map[string]promote.RecordAction {
	m := make(map[string]promote.RecordAction, len(actions))
	for _, a := range actions {
		m[a.ID] = a
	}
	return m
}

func TestPromoteCorpus_ReturnsManifestActions(t *testing.T) {
	recs := []*record.Record{
		eligibleRec("exp-0100"),               // promoted: quarantined -> validated
		eligibleRec("exp-0101"),               // held: no transition
		{ID: "exp-0102", Status: "validated"}, // ineligible: no transition
	}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true}}
	persist := func(string, *record.Record) error { return nil }

	_, actions, err := promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{}, nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("promoteCorpus: %v", err)
	}
	if len(actions) != 3 {
		t.Fatalf("want one action per record (3), got %d: %+v", len(actions), actions)
	}
	by := actionByID(actions)

	p := by["exp-0100"]
	if p.Outcome != "promoted" || p.FromStatus != "quarantined" || p.ToStatus != "validated" {
		t.Fatalf("promoted action wrong transition: %+v", p)
	}
	if p.JudgeModel != "gemini-2.5-pro" || p.JudgeDecision != string(judge.Approve) {
		t.Fatalf("promoted action missing judge provenance: %+v", p)
	}
	if p.ReproducedUnder == nil {
		t.Fatalf("promoted action missing reproduced_under: %+v", p)
	}

	h := by["exp-0101"]
	if h.Outcome != "held" || h.FromStatus != "quarantined" || h.ToStatus != "quarantined" {
		t.Fatalf("held action should record no transition: %+v", h)
	}

	in := by["exp-0102"]
	if in.Outcome != "ineligible" || in.FromStatus != "validated" || in.ToStatus != "validated" {
		t.Fatalf("ineligible action should record no transition: %+v", in)
	}
}

func TestAdaptCorpus_ReturnsManifestActions(t *testing.T) {
	orig := validatedRec("exp-0043")
	rep := reportRec("exp-0200", "exp-0043")
	recs := []*record.Record{orig, rep}
	runner := fakeCounterRunner{ev: map[string]promote.CounterEvidence{
		"exp-0043": {Original: repro.Attestation{Holds: true}, Counter: repro.Attestation{Holds: true}, CounterRepro: "x"},
	}}
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")})
	persist := func(string, *record.Record) error { return nil }

	_, actions, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{}, nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("adaptCorpus: %v", err)
	}
	by := actionByID(actions)
	d := by["exp-0043"]
	if d.Outcome != "demoted" || d.FromStatus != "validated" || d.ToStatus != "stale" {
		t.Fatalf("demote action wrong transition: %+v", d)
	}
	if d.JudgeModel != "gemini-2.5-pro" {
		t.Fatalf("demote action missing judge model: %+v", d)
	}
}
