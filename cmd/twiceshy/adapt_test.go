// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// fakeCounterRunner returns canned counter-evidence per original id.
type fakeCounterRunner struct {
	ev map[string]promote.CounterEvidence
}

func (f fakeCounterRunner) Run(_ context.Context, original, _ *record.Record) (promote.CounterEvidence, error) {
	return f.ev[original.ID], nil
}

func validatedRec(id string) *record.Record {
	rp := "experience/repro/" + id + ".sh"
	at := "2026-06-12"
	return &record.Record{
		SchemaVersion: 1, ID: id, Kind: "trap", Status: "validated",
		Title:      "a validated record with a sufficiently long title here",
		Symptom:    &record.Symptom{Summary: "x"},
		Resolution: &record.Resolution{RootCause: "x", Fix: "x"},
		Guard:      &record.Guard{Repro: &rp},
		Provenance: record.Provenance{
			Source: record.Source{Author: "a"}, RecordedAt: "2026-06-12",
			ValidatedAt: &at, Valid: record.Validity{From: "2026-06-12"},
		},
		Body: "b", Path: "experience/2026/" + id[len("exp-"):] + "-x.md",
	}
}

func reportRec(id, disputes string) *record.Record {
	d := disputes
	return &record.Record{
		SchemaVersion: 1, ID: id, Kind: "dead-end", Status: "quarantined",
		Title:      "Outcome report against " + disputes + " long enough title",
		Symptom:    &record.Symptom{Summary: "did not hold"},
		Resolution: &record.Resolution{DeadEnds: []record.DeadEnd{{Tried: "x", WhyItFailed: "y"}}},
		Provenance: record.Provenance{
			Source: record.Source{Author: "a"}, RecordedAt: "2026-06-19",
			Valid: record.Validity{From: "2026-06-19"}, Disputes: &d,
		},
		Body: "b", Path: "experience/2026/" + id[len("exp-"):] + "-r.md",
	}
}

func TestAdaptCorpus_DemotesAndPersists(t *testing.T) {
	orig := validatedRec("exp-0043")
	rep := reportRec("exp-0200", "exp-0043")
	recs := []*record.Record{orig, rep}
	runner := fakeCounterRunner{ev: map[string]promote.CounterEvidence{
		"exp-0043": {Original: repro.Attestation{Holds: true}, Counter: repro.Attestation{Holds: true}, CounterRepro: "x"},
	}}
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")})
	var persisted []string
	persist := func(_ string, r *record.Record) error { persisted = append(persisted, r.ID); return nil }

	st, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{}, nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("adaptCorpus: %v", err)
	}
	if st.demoted != 1 {
		t.Fatalf("demoted = %d, want 1", st.demoted)
	}
	if orig.Status != "stale" {
		t.Fatalf("original status = %q, want stale", orig.Status)
	}
	if len(persisted) != 1 || persisted[0] != "exp-0043" {
		t.Fatalf("persisted = %v, want [exp-0043] (the disputed record, not the report)", persisted)
	}
}

func TestAdaptCorpus_CorroboratedDisputesFlagsDisputed(t *testing.T) {
	orig := validatedRec("exp-0043")
	recs := []*record.Record{orig}
	// DisputeThreshold independent non-reproducing reports about the same record.
	for i := 0; i < promote.DisputeThreshold; i++ {
		recs = append(recs, reportRec("exp-020"+string(rune('0'+i)), "exp-0043"))
	}
	runner := fakeCounterRunner{ev: map[string]promote.CounterEvidence{
		"exp-0043": {Original: repro.Attestation{Holds: true}, Counter: repro.Attestation{Holds: false}}, // ran, did not reproduce
	}}
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("g")})
	persist := func(_ string, _ *record.Record) error { return nil }

	st, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{}, nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("adaptCorpus: %v", err)
	}
	if st.disputed < 1 || orig.Status != "disputed" {
		t.Fatalf("corroborated reports must flag disputed; stats=%+v status=%q", st, orig.Status)
	}
}

func TestAdaptCorpus_OrphanReportCounted(t *testing.T) {
	recs := []*record.Record{reportRec("exp-0200", "exp-9999")} // disputes a record not present
	runner := fakeCounterRunner{ev: map[string]promote.CounterEvidence{}}
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("g")})
	persist := func(_ string, _ *record.Record) error { return nil }

	st, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{}, nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("adaptCorpus: %v", err)
	}
	if st.orphan != 1 {
		t.Fatalf("orphan = %d, want 1", st.orphan)
	}
}

func TestRunAdapt_RequiresJudgeURL(t *testing.T) {
	var buf bytes.Buffer
	err := runAdapt(context.Background(), []string{"-corpus", "../.."}, &buf, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "TWICESHY_JUDGE_URL") {
		t.Fatalf("the counter-evidence gate without a judge must fail safe; got %v", err)
	}
}

func TestRunAdapt_DryRunWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	err := runAdapt(context.Background(), []string{"-corpus", "../..", "-dry-run"}, &buf, func(string) string { return "" })
	if err != nil {
		t.Fatalf("dry-run must not need a judge: %v", err)
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Fatalf("dry-run output missing; got %q", buf.String())
	}
}
