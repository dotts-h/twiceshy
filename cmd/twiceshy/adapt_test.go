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

// fakeCounterRunner returns canned counter-evidence per original id.
type fakeCounterRunner struct {
	ev  map[string]promote.CounterEvidence
	err map[string]error
}

func (f fakeCounterRunner) Run(_ context.Context, original, _ *record.Record) (promote.CounterEvidence, error) {
	if e, ok := f.err[original.ID]; ok {
		return promote.CounterEvidence{}, e
	}
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

	st, _, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, "")
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

	st, _, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, "")
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

	st, _, err := adaptCorpus(context.Background(), ".", recs, runner, adapter, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, "")
	if err != nil {
		t.Fatalf("adaptCorpus: %v", err)
	}
	if st.orphan != 1 {
		t.Fatalf("orphan = %d, want 1", st.orphan)
	}
}

func TestRunAdapt_RequiresJudgeURL(t *testing.T) {
	var buf bytes.Buffer
	err := runAdapt(context.Background(), []string{"-corpus", corpus}, &buf, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "TWICESHY_JUDGE_URL") {
		t.Fatalf("the counter-evidence gate without a judge must fail safe; got %v", err)
	}
}

func TestAdaptCorpus_JournalAbortsThenResumes(t *testing.T) {
	d := t.TempDir()
	jp := promote.JournalPath(d, "adapt")
	orig1 := validatedRec("exp-0043")
	orig2 := validatedRec("exp-0044")
	orig3 := validatedRec("exp-0045")
	recs := []*record.Record{
		orig1, reportRec("exp-0200", "exp-0043"),
		orig2, reportRec("exp-0201", "exp-0044"),
		orig3, reportRec("exp-0202", "exp-0045"),
	}
	ev := map[string]promote.CounterEvidence{
		"exp-0043": {Original: repro.Attestation{Holds: true}, Counter: repro.Attestation{Holds: true}, CounterRepro: "x"},
		"exp-0044": {Original: repro.Attestation{Holds: true}, Counter: repro.Attestation{Holds: true}, CounterRepro: "x"},
	}
	adapter := promote.NewAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")})
	var persisted []string
	persist := func(_ string, r *record.Record) error {
		persisted = append(persisted, r.ID)
		return nil
	}

	runner1 := fakeCounterRunner{
		ev:  ev,
		err: map[string]error{"exp-0045": errors.New("broker exploded")},
	}
	_, _, err := adaptCorpus(context.Background(), d, recs, runner1, adapter, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, jp)
	if err == nil {
		t.Fatal("first adapt run must abort on runner error")
	}
	j, lerr := promote.LoadJournal(jp)
	if lerr != nil {
		t.Fatalf("LoadJournal: %v", lerr)
	}
	if j == nil || j.Complete {
		t.Fatalf("aborted adapt journal: %+v", j)
	}
	if j.StoppedAt == nil || j.StoppedAt.RecordID != "exp-0202" {
		t.Fatalf("StoppedAt = %+v, want report exp-0202", j.StoppedAt)
	}
	done := j.DoneIDs()
	if !done["exp-0043"] || !done["exp-0044"] || done["exp-0045"] {
		t.Fatalf("DoneIDs after abort = %v, want first two originals only", done)
	}

	runner2 := fakeCounterRunner{ev: map[string]promote.CounterEvidence{
		"exp-0043": ev["exp-0043"],
		"exp-0044": ev["exp-0044"],
		"exp-0045": {Original: repro.Attestation{Holds: true}, Counter: repro.Attestation{Holds: true}, CounterRepro: "x"},
	}}
	persisted = nil
	_, _, err = adaptCorpus(context.Background(), d, recs, runner2, adapter, persist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, jp)
	if err != nil {
		t.Fatalf("resume adapt run: %v", err)
	}
	if orig1.Status != "stale" || orig2.Status != "stale" || orig3.Status != "stale" {
		t.Fatalf("status after resume: %q %q %q, want all stale", orig1.Status, orig2.Status, orig3.Status)
	}
	if len(persisted) != 1 || persisted[0] != "exp-0045" {
		t.Fatalf("persisted on resume = %v, want only exp-0045", persisted)
	}
	j, lerr = promote.LoadJournal(jp)
	if lerr != nil {
		t.Fatalf("LoadJournal after resume: %v", lerr)
	}
	if !j.Complete {
		t.Fatal("completed adapt resume must mark journal complete")
	}
}

func TestRunAdapt_DryRunWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	err := runAdapt(context.Background(), []string{"-corpus", corpus, "-dry-run"}, &buf, func(string) string { return "" })
	if err != nil {
		t.Fatalf("dry-run must not need a judge: %v", err)
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Fatalf("dry-run output missing; got %q", buf.String())
	}
}
