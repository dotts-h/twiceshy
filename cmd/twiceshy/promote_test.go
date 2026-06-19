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

// fakePromoter is a stub recordPromoter: it promotes the ids in `promote`,
// errors for ids in `err`, and otherwise holds. On a promotion it mutates the
// record like the real promoter so persist sees the flip.
type fakePromoter struct {
	promote map[string]bool
	err     map[string]error
	calls   []string
}

func (f *fakePromoter) Promote(_ context.Context, rec *record.Record) (promote.Outcome, error) {
	f.calls = append(f.calls, rec.ID)
	if e, ok := f.err[rec.ID]; ok {
		return promote.Outcome{}, e
	}
	if f.promote[rec.ID] {
		rec.Status = "validated"
		return promote.Outcome{
			Promoted:    true,
			Verdict:     judge.Verdict{Model: "gemini-2.5-pro", Decision: judge.Approve},
			Attestation: repro.Attestation{ReproducedUnder: []string{"go1.25"}},
		}, nil
	}
	return promote.Outcome{Reason: "judge did not approve"}, nil
}

func eligibleRec(id string) *record.Record {
	rp := "experience/repro/" + id + ".sh"
	return &record.Record{
		ID: id, Status: "quarantined",
		Guard: &record.Guard{Repro: &rp},
		Path:  "experience/2026/" + id[len("exp-"):] + "-x.md",
	}
}

func TestPromoteCorpus_PromotesEligiblePersistsFlip(t *testing.T) {
	recs := []*record.Record{
		eligibleRec("exp-0100"),                 // promoted
		eligibleRec("exp-0101"),                 // held
		{ID: "exp-0102", Status: "validated"},   // ineligible (not quarantined)
		{ID: "exp-0103", Status: "quarantined"}, // ineligible (no repro)
	}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true}}
	var persisted []string
	persist := func(_ string, rec *record.Record) error {
		persisted = append(persisted, rec.ID)
		return nil
	}
	var buf bytes.Buffer

	st, err := promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{}, nil, &buf)
	if err != nil {
		t.Fatalf("promoteCorpus: %v", err)
	}
	if st.promoted != 1 || st.held != 1 || st.ineligible != 2 {
		t.Fatalf("stats = %+v, want promoted 1 / held 1 / ineligible 2", st)
	}
	if len(persisted) != 1 || persisted[0] != "exp-0100" {
		t.Fatalf("persisted = %v, want [exp-0100] (only the promoted record is written)", persisted)
	}
	// Ineligible records never reach the promoter (cost guard).
	if strings.Join(fp.calls, ",") != "exp-0100,exp-0101" {
		t.Fatalf("promoter calls = %v, want only the two eligible records", fp.calls)
	}
}

func TestPromoteCorpus_AbortsOnPromoterError(t *testing.T) {
	recs := []*record.Record{eligibleRec("exp-0100"), eligibleRec("exp-0101")}
	fp := &fakePromoter{
		promote: map[string]bool{"exp-0100": true},
		err:     map[string]error{"exp-0101": errors.New("broker exploded")},
	}
	var persisted []string
	persist := func(_ string, rec *record.Record) error { persisted = append(persisted, rec.ID); return nil }

	_, err := promoteCorpus(context.Background(), ".", recs, fp, persist, guard.Guardrails{}, nil, &bytes.Buffer{})
	if err == nil {
		t.Fatal("a promoter error must abort the run")
	}
	// The record promoted before the error stays written (independently valid).
	if len(persisted) != 1 || persisted[0] != "exp-0100" {
		t.Fatalf("persisted = %v, want [exp-0100]", persisted)
	}
}

func TestRunPromote_RequiresJudgeURL(t *testing.T) {
	var buf bytes.Buffer
	err := runPromote(context.Background(), []string{"-corpus", "../.."}, &buf,
		func(string) string { return "" }) // no TWICESHY_JUDGE_URL
	if err == nil || !strings.Contains(err.Error(), "TWICESHY_JUDGE_URL") {
		t.Fatalf("auto-promotion without a judge must fail safe; got %v", err)
	}
}

func TestRunPromote_RejectsLocalJudgeModel(t *testing.T) {
	var buf bytes.Buffer
	err := runPromote(context.Background(), []string{"-corpus", "../..", "-judge-model", "llama3.2"}, &buf,
		func(k string) string {
			if k == "TWICESHY_JUDGE_URL" {
				return "http://judge.local"
			}
			return ""
		})
	if err == nil || !strings.Contains(err.Error(), "judge") {
		t.Fatalf("the cheap local model must be rejected as judge; got %v", err)
	}
}

func TestRunPromote_DryRunWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	err := runPromote(context.Background(), []string{"-corpus", "../..", "-dry-run"}, &buf,
		func(string) string { return "" })
	if err != nil {
		t.Fatalf("dry-run must not need a judge: %v", err)
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Fatalf("dry-run output missing; got %q", buf.String())
	}
}
