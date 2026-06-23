// SPDX-License-Identifier: AGPL-3.0-only

package promote_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// demotedRecord is a stale trap carrying valid.until + a demotion audit block —
// the execution-provable class eligible for re-promotion.
func demotedRecord() *record.Record {
	rp := "experience/repro/0043.sh"
	until := "2026-06-18"
	validatedAt := "2026-06-01"
	return &record.Record{
		SchemaVersion: 1, ID: "exp-0043", Kind: "trap", Status: "stale",
		Title:   "io/ioutil deprecated — ReadAll moved in Go 1.16, long enough title",
		Symptom: &record.Symptom{Summary: "ioutil.ReadAll is deprecated"},
		Resolution: &record.Resolution{
			RootCause: "ioutil was redistributed in Go 1.16",
			Fix:       "use io.ReadAll",
		},
		Guard:     &record.Guard{Repro: &rp},
		AppliesTo: []record.AppliesTo{{Ecosystem: "Go", Package: "io/ioutil"}},
		Provenance: record.Provenance{
			Source: record.Source{Author: "agent"}, RecordedAt: "2026-06-01",
			ValidatedAt: &validatedAt,
			Valid:       record.Validity{From: "2026-06-01", Until: &until},
			Demotion: &record.Demotion{
				AttestedAt: "2026-06-18T00:00:00Z", JudgeModel: "gemini-2.5-pro",
				JudgeDecision: "approve", Report: "exp-0099",
			},
		},
		Body: "The repro builds a package importing io/ioutil and proves the deprecation.",
		Path: "experience/2026/0043-ioutil.md",
	}
}

func disputedRecord() *record.Record {
	rec := demotedRecord()
	rec.Status = "disputed"
	rec.Provenance.Valid.Until = nil
	rec.Provenance.Demotion = nil
	return rec
}

func untilCleared(u *string) bool { return u == nil || *u == "" }

func TestRepromote_HoldingPlusApprove_RestoresStale(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, j)
	rec := demotedRecord()

	out, err := p.Repromote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Repromote: %v", err)
	}
	if !out.Promoted {
		t.Fatalf("expected re-promotion, got reason %q", out.Reason)
	}
	if rec.Status != "validated" {
		t.Fatalf("status = %q, want validated", rec.Status)
	}
	if rec.Provenance.ValidatedAt == nil || *rec.Provenance.ValidatedAt != "2026-06-19" {
		t.Fatalf("validated_at = %v, want 2026-06-19", rec.Provenance.ValidatedAt)
	}
	if !untilCleared(rec.Provenance.Valid.Until) {
		t.Fatalf("valid.until = %v, want cleared", rec.Provenance.Valid.Until)
	}
	if rec.Provenance.Demotion != nil {
		t.Fatalf("demotion block must be cleared, got %+v", rec.Provenance.Demotion)
	}
	pr := rec.Provenance.Promotion
	if pr == nil || pr.JudgeModel != "gemini-2.5-pro" || pr.JudgeDecision != "approve" || pr.AttestedAt != "2026-06-19T00:00:00Z" {
		t.Fatalf("promotion audit block wrong: %+v", pr)
	}
	if err := record.Validate(rec); err != nil {
		t.Fatalf("re-promoted record must be schema-valid: %v", err)
	}
}

func TestRepromote_HoldingPlusApprove_RestoresDisputed(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, j)
	rec := disputedRecord()

	out, err := p.Repromote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Repromote: %v", err)
	}
	if !out.Promoted {
		t.Fatalf("expected re-promotion, got reason %q", out.Reason)
	}
	if rec.Status != "validated" {
		t.Fatalf("status = %q, want validated", rec.Status)
	}
	if !untilCleared(rec.Provenance.Valid.Until) {
		t.Fatalf("valid.until = %v, want cleared", rec.Provenance.Valid.Until)
	}
	if rec.Provenance.Demotion != nil {
		t.Fatalf("demotion block must be cleared, got %+v", rec.Provenance.Demotion)
	}
	if rec.Provenance.Promotion == nil {
		t.Fatal("promotion audit block must be set")
	}
}

func TestRepromote_JudgeReject_StaysDemoted(t *testing.T) {
	j := &judge.StubJudge{Verdict: judge.Verdict{Decision: judge.Reject}}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, j)
	rec := demotedRecord()
	orig := *rec

	out, err := p.Repromote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Repromote: %v", err)
	}
	if out.Promoted {
		t.Fatal("a rejected verdict must not re-promote")
	}
	if rec.Status != orig.Status || rec.Provenance.Demotion == nil || rec.Provenance.Promotion != nil {
		t.Fatalf("record mutated on reject: status=%q demotion=%+v promotion=%+v", rec.Status, rec.Provenance.Demotion, rec.Provenance.Promotion)
	}
}

func TestRepromoteEligible_NotDemoted_Skipped(t *testing.T) {
	// The security-flagged and no-repro bases use demotedRecord() (status "stale",
	// Guard populated, no flags) so they hit their OWN reject branch, not the
	// already-tested not-demoted branch. The security-flagged case guards the core
	// invariant that a flagged record can NEVER be restored to validated.
	cases := map[string]struct {
		mk     func() *record.Record
		reason string
	}{
		"validated":   {func() *record.Record { r := provableRecord(); r.Status = "validated"; return r }, "not a demoted record"},
		"quarantined": {provableRecord, "not a demoted record"},
		"security-flagged": {func() *record.Record {
			r := demotedRecord()
			r.Provenance.SecurityFlags = []string{"secret:aws-access-key"}
			return r
		}, "security-flagged record cannot be validated"},
		"no-repro": {func() *record.Record {
			r := demotedRecord()
			r.Guard = nil
			return r
		}, "no executable proof — left for a human (ADR-0013 §5)"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ok, reason := promote.RepromoteEligible(c.mk())
			if ok {
				t.Fatalf("%s must not be re-promotable", name)
			}
			if reason != c.reason {
				t.Fatalf("reason = %q, want %q", reason, c.reason)
			}
		})
	}
}

func TestRepromote_NonHoldingOrInconclusive_StaysDemoted(t *testing.T) {
	for name, att := range map[string]repro.Attestation{
		"not holding":  {RecordID: "exp-0043", Holds: false},
		"inconclusive": {RecordID: "exp-0043", Holds: true, Inconclusive: true},
	} {
		t.Run(name, func(t *testing.T) {
			j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
			p := newPromoter(t, &stubAttestor{att: att}, j)
			rec := demotedRecord()
			origStatus := rec.Status
			out, _ := p.Repromote(context.Background(), rec)
			if out.Promoted || rec.Status != origStatus {
				t.Fatalf("%s attestation must not re-promote", name)
			}
			if j.last.Record != nil {
				t.Fatal("the judge must not even be consulted without a holding attestation (cost guard)")
			}
		})
	}
}

// Repromote mutates the demoted record to validated, then validates; if the
// re-promoted record is invalid it MUST revert every mutated field (status,
// valid.until, validated_at, demotion, promotion) and return a hard error so the
// caller never persists a half-promoted record (promote.go:415-422). badClock()
// forces record.Validate to red on provenance.validated_at without any product
// change.
func TestRepromote_InvalidPromotedRecord_HardErrorAndReverts(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, j, badClock())
	rec := demotedRecord()
	origStatus := rec.Status
	origUntil := rec.Provenance.Valid.Until
	origValidatedAt := rec.Provenance.ValidatedAt
	origDemotion := rec.Provenance.Demotion
	origPromotion := rec.Provenance.Promotion

	out, err := p.Repromote(context.Background(), rec)
	if err == nil {
		t.Fatal("an invalid re-promoted record must be a hard error, not a silent skip")
	}
	if !strings.Contains(err.Error(), "re-promoted record is invalid") {
		t.Fatalf("error must explain the not-persisted revert, got %q", err.Error())
	}
	if out.Promoted {
		t.Fatal("an invalid re-promoted record must not report Promoted")
	}
	if rec.Status != origStatus {
		t.Fatalf("status not reverted: %q, want %q", rec.Status, origStatus)
	}
	if rec.Provenance.Valid.Until != origUntil {
		t.Fatalf("valid.until not reverted: %v, want %v", rec.Provenance.Valid.Until, origUntil)
	}
	if rec.Provenance.ValidatedAt != origValidatedAt {
		t.Fatalf("validated_at not reverted: %v, want %v", rec.Provenance.ValidatedAt, origValidatedAt)
	}
	if rec.Provenance.Demotion != origDemotion {
		t.Fatalf("demotion block not reverted: %+v, want %+v", rec.Provenance.Demotion, origDemotion)
	}
	if rec.Provenance.Promotion != origPromotion {
		t.Fatalf("promotion block not reverted: %+v, want %+v", rec.Provenance.Promotion, origPromotion)
	}
}

func TestRepromote_AttestorError_IsHardError(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	p := newPromoter(t, &stubAttestor{err: errors.New("broker exploded")}, j)
	rec := demotedRecord()
	if _, err := p.Repromote(context.Background(), rec); err == nil {
		t.Fatal("an attestor (broker) error must surface as an error, not a silent skip")
	}
}
