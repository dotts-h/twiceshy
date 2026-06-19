// SPDX-License-Identifier: AGPL-3.0-only

package promote_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/doctor"
	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// stubAttestor returns a preset attestation (the revalidate doctor's role),
// so the promoter can be exercised with no broker / Docker.
type stubAttestor struct {
	att repro.Attestation
	err error
}

func (s stubAttestor) RunWithAttestations(_ context.Context, _ []*record.Record) (doctor.Report, []repro.Attestation, error) {
	if s.err != nil {
		return doctor.Report{}, nil, s.err
	}
	return doctor.Report{}, []repro.Attestation{s.att}, nil
}

// captureJudge records the request it was handed so a test can assert the
// promoter passes the proof through.
type captureJudge struct {
	last    judge.Request
	verdict judge.Verdict
	err     error
}

func (c *captureJudge) Judge(_ context.Context, req judge.Request) (judge.Verdict, error) {
	c.last = req
	return c.verdict, c.err
}

func holdingAtt() repro.Attestation {
	return repro.Attestation{RecordID: "exp-0043", RanAt: "2026-06-19T00:00:00Z", Holds: true, Inconclusive: false, ReproducedUnder: []string{"go1.25"}}
}

// provableRecord is a quarantined trap carrying a positive repro — the
// execution-provable class eligible for auto-promotion.
func provableRecord() *record.Record {
	rp := "experience/repro/0043.sh"
	return &record.Record{
		SchemaVersion: 1, ID: "exp-0043", Kind: "trap", Status: "quarantined",
		Title:   "io/ioutil deprecated — ReadAll moved in Go 1.16, long enough title",
		Symptom: &record.Symptom{Summary: "ioutil.ReadAll is deprecated"},
		Resolution: &record.Resolution{
			RootCause: "ioutil was redistributed in Go 1.16",
			Fix:       "use io.ReadAll",
		},
		Guard:     &record.Guard{Repro: &rp},
		AppliesTo: []record.AppliesTo{{Ecosystem: "Go", Package: "io/ioutil"}},
		Provenance: record.Provenance{
			Source: record.Source{Author: "agent"}, RecordedAt: "2026-06-19",
			Valid: record.Validity{From: "2026-06-19"},
		},
		Body: "The repro builds a package importing io/ioutil and proves the deprecation.",
		Path: "experience/2026/0043-ioutil.md",
	}
}

func newPromoter(t *testing.T, att stubAttestor, j judge.Judge) *promote.Promoter {
	t.Helper()
	return promote.NewPromoter(att, j, ".",
		promote.WithReproReader(func(string) (string, error) { return "#!/bin/sh\ntrue", nil }),
		promote.WithClock(func() string { return "2026-06-19" }),
	)
}

func TestPromote_HoldingPlusApprove_Promotes(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	p := newPromoter(t, stubAttestor{att: holdingAtt()}, j)
	rec := provableRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !out.Promoted {
		t.Fatalf("expected promotion, got reason %q", out.Reason)
	}
	if rec.Status != "validated" {
		t.Fatalf("status = %q, want validated", rec.Status)
	}
	if rec.Provenance.ValidatedAt == nil || *rec.Provenance.ValidatedAt != "2026-06-19" {
		t.Fatalf("validated_at = %v, want 2026-06-19", rec.Provenance.ValidatedAt)
	}
	pr := rec.Provenance.Promotion
	if pr == nil || pr.JudgeModel != "gemini-2.5-pro" || pr.JudgeDecision != "approve" || pr.AttestedAt != "2026-06-19T00:00:00Z" {
		t.Fatalf("promotion audit block wrong: %+v", pr)
	}
	if len(pr.ReproducedUnder) != 1 || pr.ReproducedUnder[0] != "go1.25" {
		t.Fatalf("reproduced_under = %+v, want [go1.25]", pr.ReproducedUnder)
	}
	if err := record.Validate(rec); err != nil {
		t.Fatalf("promoted record must be schema-valid: %v", err)
	}
	// The judge must have seen the holding attestation + the repro content.
	if j.last.Attestation.RecordID != "exp-0043" || len(j.last.Repros) != 1 || j.last.Repros[0].Content == "" {
		t.Fatalf("judge did not receive the proof: %+v", j.last)
	}
}

func TestPromote_JudgeReject_StaysQuarantined(t *testing.T) {
	j := &judge.StubJudge{Verdict: judge.Verdict{Decision: judge.Reject}}
	p := newPromoter(t, stubAttestor{att: holdingAtt()}, j)
	rec := provableRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if out.Promoted {
		t.Fatal("a rejected verdict must not promote")
	}
	if rec.Status != "quarantined" || rec.Provenance.Promotion != nil {
		t.Fatalf("record mutated on reject: status=%q promotion=%+v", rec.Status, rec.Provenance.Promotion)
	}
}

func TestPromote_JudgeError_FailSafe(t *testing.T) {
	j := &judge.StubJudge{Err: errors.New("judge endpoint down")}
	p := newPromoter(t, stubAttestor{att: holdingAtt()}, j)
	rec := provableRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("a judge outage is fail-safe, not a hard error: %v", err)
	}
	if out.Promoted || rec.Status != "quarantined" {
		t.Fatal("a judge outage must leave the record quarantined (fail-safe)")
	}
}

func TestPromote_NonHoldingOrInconclusive_StaysQuarantined(t *testing.T) {
	for name, att := range map[string]repro.Attestation{
		"not holding":  {RecordID: "exp-0043", Holds: false},
		"inconclusive": {RecordID: "exp-0043", Holds: true, Inconclusive: true},
	} {
		t.Run(name, func(t *testing.T) {
			j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
			p := newPromoter(t, stubAttestor{att: att}, j)
			rec := provableRecord()
			out, _ := p.Promote(context.Background(), rec)
			if out.Promoted || rec.Status != "quarantined" {
				t.Fatalf("%s attestation must not promote", name)
			}
			if j.last.Record != nil {
				t.Fatal("the judge must not even be consulted without a holding attestation (cost guard)")
			}
		})
	}
}

func TestPromote_IneligibleRecords_Skipped(t *testing.T) {
	disputed := "exp-0001"
	cases := map[string]func(*record.Record){
		"already validated":    func(r *record.Record) { r.Status = "validated"; at := "2026-06-19"; r.Provenance.ValidatedAt = &at },
		"is an outcome report": func(r *record.Record) { r.Provenance.Disputes = &disputed },
		"security flagged":     func(r *record.Record) { r.Provenance.SecurityFlags = []string{"secret:aws-access-key"} },
		"no executable proof":  func(r *record.Record) { r.Guard = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
			p := newPromoter(t, stubAttestor{att: holdingAtt()}, j)
			rec := provableRecord()
			mutate(rec)
			out, err := p.Promote(context.Background(), rec)
			if err != nil {
				t.Fatalf("Promote: %v", err)
			}
			if out.Promoted {
				t.Fatalf("%s must be skipped, not promoted", name)
			}
			if j.last.Record != nil {
				t.Fatalf("%s: the judge must not be consulted for an ineligible record", name)
			}
		})
	}
}

func TestPromote_AttestorError_IsHardError(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	p := newPromoter(t, stubAttestor{err: errors.New("broker exploded")}, j)
	rec := provableRecord()
	if _, err := p.Promote(context.Background(), rec); err == nil {
		t.Fatal("an attestor (broker) error must surface as an error, not a silent skip")
	}
}

func writeRepro(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Exercises the default repro reader: a file repro and a directory repro (its
// repro.sh) are both resolved from the corpus root and handed to the judge.
func TestPromote_DefaultReproReader_FileAndDir(t *testing.T) {
	for name, rel := range map[string]string{
		"file repro": "experience/repro/x.sh",
		"dir repro":  "experience/repro/d", // resolves to d/repro.sh
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeRepro(t, root, "experience/repro/x.sh", "#!/bin/sh\necho hello-file")
			writeRepro(t, root, "experience/repro/d/repro.sh", "#!/bin/sh\necho hello-dir")

			j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
			p := promote.NewPromoter(stubAttestor{att: holdingAtt()}, j, root,
				promote.WithClock(func() string { return "2026-06-19" }))
			rec := provableRecord()
			path := rel
			rec.Guard.Repro = &path

			if _, err := p.Promote(context.Background(), rec); err != nil {
				t.Fatalf("Promote: %v", err)
			}
			if len(j.last.Repros) != 1 || !strings.Contains(j.last.Repros[0].Content, "hello") {
				t.Fatalf("judge did not get the repro content: %+v", j.last.Repros)
			}
		})
	}
}

// A record carrying BOTH the legacy guard.repro and a guard.repros test-set
// must hand the judge every script (the proof body), in order.
func TestPromote_MultipleRepros_AllReachJudge(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	p := promote.NewPromoter(stubAttestor{att: holdingAtt()}, j, ".",
		promote.WithReproReader(func(rel string) (string, error) { return "content-of:" + rel, nil }),
		promote.WithClock(func() string { return "2026-06-19" }))
	rec := provableRecord()
	rec.Guard.Repros = []record.Repro{
		{Path: "experience/repro/extra-neg.sh", Kind: "negative", Label: "must stay failing"},
	}
	if _, err := p.Promote(context.Background(), rec); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if len(j.last.Repros) != 2 {
		t.Fatalf("judge got %d repros, want 2 (the positive guard.repro + the guard.repros entry)", len(j.last.Repros))
	}
	if j.last.Repros[0].Kind != "positive" || j.last.Repros[1].Kind != "negative" {
		t.Fatalf("repro kinds/order wrong: %+v", j.last.Repros)
	}
	if j.last.Repros[1].Content != "content-of:experience/repro/extra-neg.sh" {
		t.Fatalf("guard.repros content not read: %q", j.last.Repros[1].Content)
	}
}

func TestPromote_ReproPathEscape_IsHardError(t *testing.T) {
	root := t.TempDir()
	j := &judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	p := promote.NewPromoter(stubAttestor{att: holdingAtt()}, j, root,
		promote.WithClock(func() string { return "2026-06-19" }))
	rec := provableRecord()
	esc := "../escape.sh"
	rec.Guard.Repro = &esc

	if _, err := p.Promote(context.Background(), rec); err == nil {
		t.Fatal("a repro path escaping the corpus root must be a hard error, never read")
	}
	if rec.Status != "quarantined" {
		t.Fatal("a failed repro read must not promote")
	}
}
