// SPDX-License-Identifier: AGPL-3.0-only

package promote_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// validatedOriginal is a served record an outcome report disputes.
func validatedOriginal() *record.Record {
	rp := "experience/repro/0043.sh"
	at := "2026-06-12"
	return &record.Record{
		SchemaVersion: 1, ID: "exp-0043", Kind: "trap", Status: "validated",
		Title:      "io/ioutil deprecated — ReadAll moved in Go 1.16, long enough title",
		Symptom:    &record.Symptom{Summary: "ioutil.ReadAll is deprecated"},
		Resolution: &record.Resolution{RootCause: "redistributed in 1.16", Fix: "use io.ReadAll"},
		Guard:      &record.Guard{Repro: &rp},
		AppliesTo:  []record.AppliesTo{{Ecosystem: "Go", Package: "io/ioutil"}},
		Provenance: record.Provenance{
			Source: record.Source{Author: "agent"}, RecordedAt: "2026-06-12",
			ValidatedAt: &at, Valid: record.Validity{From: "2026-06-12"},
		},
		Body: "Proven deprecation.",
		Path: "experience/2026/0043-ioutil.md",
	}
}

// reportFor builds a quarantined counter-record disputing orig.
func reportFor(orig string) *record.Record {
	d := orig
	return &record.Record{
		SchemaVersion: 1, ID: "exp-0200", Kind: "dead-end", Status: "quarantined",
		Title:      "Outcome report against " + orig + " — reported failed, long enough",
		Symptom:    &record.Symptom{Summary: "the lesson did not hold"},
		Resolution: &record.Resolution{DeadEnds: []record.DeadEnd{{Tried: "followed " + orig, WhyItFailed: "still errors"}}},
		Provenance: record.Provenance{
			Source: record.Source{Author: "agent"}, RecordedAt: "2026-06-19",
			Valid: record.Validity{From: "2026-06-19"}, Disputes: &d,
		},
		Body: "report body",
		Path: "experience/2026/0200-report.md",
	}
}

func holds(id string) repro.Attestation {
	return repro.Attestation{RecordID: id, RanAt: "2026-06-19T00:00:00Z", Holds: true}
}
func broke(id string) repro.Attestation {
	return repro.Attestation{RecordID: id, RanAt: "2026-06-19T00:00:00Z", Holds: false} // ran, did not hold
}
func inconclusive(id string) repro.Attestation {
	return repro.Attestation{RecordID: id, RanAt: "2026-06-19T00:00:00Z", Holds: false, Inconclusive: true}
}

func newAdapter(j judge.Judge) *promote.Adapter {
	return promote.NewAdapter(j, promote.WithAdaptClock(func() string { return "2026-06-19" }))
}

func TestAdapt_CounterReproduces_JudgeApprove_DemotesToStale(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	a := newAdapter(j)
	orig, rep := validatedOriginal(), reportFor("exp-0043")
	ev := promote.CounterEvidence{Original: holds("exp-0043"), Counter: holds("counter"), CounterRepro: "#!/bin/sh\n# fails"}

	out, err := a.Adapt(context.Background(), orig, rep, ev, 0)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	if out.Action != promote.ActionDemote {
		t.Fatalf("action = %q, want demote (counter reproduced)", out.Action)
	}
	if orig.Status != "stale" {
		t.Fatalf("status = %q, want stale", orig.Status)
	}
	if orig.Provenance.Valid.Until == nil || *orig.Provenance.Valid.Until != "2026-06-19" {
		t.Fatalf("valid.until = %v, want 2026-06-19", orig.Provenance.Valid.Until)
	}
	d := orig.Provenance.Demotion
	if d == nil || d.Report != "exp-0200" || d.JudgeModel != "gemini-2.5-pro" || d.JudgeDecision != "approve" {
		t.Fatalf("demotion audit wrong: %+v", d)
	}
	if err := record.Validate(orig); err != nil {
		t.Fatalf("demoted record must be schema-valid: %v", err)
	}
	// The judge must have seen the counter-evidence (the materialized repro).
	if len(j.last.Repros) != 1 || j.last.Repros[0].Content == "" {
		t.Fatalf("judge did not receive the counter-repro: %+v", j.last.Repros)
	}
}

func TestAdapt_OriginalBroke_JudgeApprove_Demotes(t *testing.T) {
	a := newAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("g")})
	orig, rep := validatedOriginal(), reportFor("exp-0043")
	// counter didn't reproduce, but the original's own repro no longer holds.
	ev := promote.CounterEvidence{Original: broke("exp-0043"), Counter: broke("counter"), CounterRepro: "x"}

	out, _ := a.Adapt(context.Background(), orig, rep, ev, 0)
	if out.Action != promote.ActionDemote || orig.Status != "stale" {
		t.Fatalf("a broken original repro must demote; got %q / %q", out.Action, orig.Status)
	}
}

func TestAdapt_Reproduced_JudgeReject_NoChange(t *testing.T) {
	a := newAdapter(&judge.StubJudge{Verdict: judge.Verdict{Decision: judge.Reject}})
	orig, rep := validatedOriginal(), reportFor("exp-0043")
	ev := promote.CounterEvidence{Original: holds("exp-0043"), Counter: holds("counter"), CounterRepro: "x"}

	out, _ := a.Adapt(context.Background(), orig, rep, ev, 0)
	if out.Action != promote.ActionNone || orig.Status != "validated" {
		t.Fatalf("a judge reject must not demote; got %q / %q", out.Action, orig.Status)
	}
}

func TestAdapt_Reproduced_JudgeError_FailSafe(t *testing.T) {
	a := newAdapter(&judge.StubJudge{Err: errors.New("judge down")})
	orig, rep := validatedOriginal(), reportFor("exp-0043")
	ev := promote.CounterEvidence{Original: holds("exp-0043"), Counter: holds("counter"), CounterRepro: "x"}

	out, err := a.Adapt(context.Background(), orig, rep, ev, 0)
	if err != nil {
		t.Fatalf("a judge outage is fail-safe, not a hard error: %v", err)
	}
	if out.Action != promote.ActionNone || orig.Status != "validated" {
		t.Fatal("a judge outage must leave the record validated (fail-safe)")
	}
}

func TestAdapt_NotReproduced_BelowThreshold_Accumulates(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("g")}
	a := newAdapter(j)
	orig, rep := validatedOriginal(), reportFor("exp-0043")
	ev := promote.CounterEvidence{Original: holds("exp-0043"), Counter: broke("counter"), CounterRepro: "x"}

	out, _ := a.Adapt(context.Background(), orig, rep, ev, 0) // 0 corroborating + this = 1 < threshold
	if out.Action != promote.ActionNone || orig.Status != "validated" {
		t.Fatalf("below-threshold non-reproducing report must not change the card; got %q / %q", out.Action, orig.Status)
	}
	if j.last.Record != nil {
		t.Fatal("the judge must NOT be consulted for a non-reproducing report (it can't demote a correct card)")
	}
}

func TestAdapt_NotReproduced_Corroborated_FlagsDisputed(t *testing.T) {
	a := newAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("g")})
	orig, rep := validatedOriginal(), reportFor("exp-0043")
	ev := promote.CounterEvidence{Original: holds("exp-0043"), Counter: broke("counter"), CounterRepro: "x"}

	// corroborating = threshold-1 others + this report reaches the threshold.
	out, err := a.Adapt(context.Background(), orig, rep, ev, promote.DisputeThreshold-1)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	if out.Action != promote.ActionDispute || orig.Status != "disputed" {
		t.Fatalf("corroborated non-reproducing reports must flag disputed; got %q / %q", out.Action, orig.Status)
	}
	if err := record.Validate(orig); err != nil {
		t.Fatalf("disputed record must be schema-valid: %v", err)
	}
}

func TestAdapt_CounterInconclusive_NoSignal(t *testing.T) {
	a := newAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("g")})
	orig, rep := validatedOriginal(), reportFor("exp-0043")
	// original still holds, counter couldn't run → no signal either way, even with corroboration.
	ev := promote.CounterEvidence{Original: holds("exp-0043"), Counter: inconclusive("counter"), CounterRepro: "x"}

	out, _ := a.Adapt(context.Background(), orig, rep, ev, promote.DisputeThreshold+5)
	if out.Action != promote.ActionNone || orig.Status != "validated" {
		t.Fatalf("an inconclusive counter must not dispute or demote; got %q / %q", out.Action, orig.Status)
	}
}

func TestAdapt_NonValidatedOriginal_Skipped(t *testing.T) {
	a := newAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("g")})
	orig, rep := validatedOriginal(), reportFor("exp-0043")
	orig.Status = "stale" // already demoted
	ev := promote.CounterEvidence{Original: holds("exp-0043"), Counter: holds("counter"), CounterRepro: "x"}

	out, _ := a.Adapt(context.Background(), orig, rep, ev, 0)
	if out.Action != promote.ActionNone {
		t.Fatalf("a non-validated original must be skipped; got %q", out.Action)
	}
}

func TestAdapt_MismatchedReport_IsError(t *testing.T) {
	a := newAdapter(&judge.StubJudge{Verdict: judge.ApproveVerdict("g")})
	orig := validatedOriginal()
	rep := reportFor("exp-9999") // disputes a different record
	ev := promote.CounterEvidence{Original: holds("exp-0043"), Counter: holds("counter"), CounterRepro: "x"}

	if _, err := a.Adapt(context.Background(), orig, rep, ev, 0); err == nil {
		t.Fatal("a report disputing a different record than the original is a caller bug — must error")
	}
}
