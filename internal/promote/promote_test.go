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
	att   repro.Attestation
	err   error
	Calls int
}

func (s *stubAttestor) RunWithAttestations(_ context.Context, _ []*record.Record) (doctor.Report, []repro.Attestation, error) {
	s.Calls++
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

func newPromoter(t *testing.T, att *stubAttestor, j judge.Judge, opts ...promote.Option) *promote.Promoter {
	t.Helper()
	base := []promote.Option{
		promote.WithReproReader(func(string) (string, error) { return "#!/bin/sh\ntrue", nil }),
		promote.WithClock(func() string { return "2026-06-19" }),
	}
	return promote.NewPromoter(att, j, ".", append(base, opts...)...)
}

func advisoryRecord() *record.Record {
	return &record.Record{
		SchemaVersion: 1, ID: "exp-0007", Kind: "trap", Status: "quarantined",
		Title: "GHSA advisory long enough title here",
		Symptom: &record.Symptom{
			Summary:         "known vulnerability",
			ErrorSignatures: []string{"GHSA-227x-7mh8-3cf6"},
		},
		AppliesTo: []record.AppliesTo{{Ecosystem: "Go", Package: "example.com/pkg"}},
		Resolution: &record.Resolution{
			RootCause: "Known vulnerability per OSV.",
			Fix:       "Upgrade past the fixed version.",
		},
		Provenance: record.Provenance{
			Source: record.Source{Author: "twiceshy-importer"}, RecordedAt: "2026-06-18",
			Valid: record.Validity{From: "2026-06-18"}, SourceLicense: "CC-BY-4.0",
			SourceURL: "https://example.com/advisory",
		},
		Body: "OSV advisory body long enough to validate.",
		Path: "experience/2026/0007-ghsa.md",
	}
}

func TestPromote_HoldingPlusApprove_Promotes(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, j)
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
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, j)
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
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, j)
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
			p := newPromoter(t, &stubAttestor{att: att}, j)
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
			p := newPromoter(t, &stubAttestor{att: holdingAtt()}, j)
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
	p := newPromoter(t, &stubAttestor{err: errors.New("broker exploded")}, j)
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
			p := promote.NewPromoter(&stubAttestor{att: holdingAtt()}, j, root,
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
	p := promote.NewPromoter(&stubAttestor{att: holdingAtt()}, j, ".",
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
	p := promote.NewPromoter(&stubAttestor{att: holdingAtt()}, j, root,
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

func TestPromote_AdvisoryPanelApproves_PromotesWithoutAttestor(t *testing.T) {
	att := &stubAttestor{att: holdingAtt()}
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gpt-oss:20b")}},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")}},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	proofJudge := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	p := newPromoter(t, att, proofJudge, promote.WithAdvisoryPanel(panel))
	rec := advisoryRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !out.Promoted {
		t.Fatalf("expected advisory promotion, got reason %q", out.Reason)
	}
	if att.Calls != 0 {
		t.Fatalf("attestor must not be called on advisory path; calls=%d", att.Calls)
	}
	if proofJudge.last.Record != nil {
		t.Fatal("proof judge must not be consulted for advisory records")
	}
	if rec.Status != "validated" {
		t.Fatalf("status = %q, want validated", rec.Status)
	}
	pr := rec.Provenance.Promotion
	if pr == nil || pr.AttestedAt != "" || len(pr.Panel) != 2 {
		t.Fatalf("promotion audit block wrong: %+v", pr)
	}
	if pr.JudgeModel != "gpt-oss:20b+gemini-2.5-pro" || pr.JudgeDecision != "approve" {
		t.Fatalf("top-level promotion fields wrong: %+v", pr)
	}
	if err := record.Validate(rec); err != nil {
		t.Fatalf("promoted advisory must validate: %v", err)
	}
}

func TestPromote_AdvisoryPanelRejects_StaysQuarantined(t *testing.T) {
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gpt-oss:20b")}},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: &judge.StubJudge{Verdict: judge.Verdict{Decision: judge.Reject}}},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{}, promote.WithAdvisoryPanel(panel))
	rec := advisoryRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if out.Promoted || rec.Status != "quarantined" {
		t.Fatal("one panel dissent must not promote")
	}
}

func TestPromote_AdvisoryNoPanel_Skips(t *testing.T) {
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{})
	rec := advisoryRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if out.Promoted {
		t.Fatal("advisory without panel must not promote")
	}
	if out.Reason != "no advisory panel configured — left for a human" {
		t.Fatalf("reason = %q", out.Reason)
	}
}

// proseRecord is a quarantined prose lesson — NO repro and NO vuln id: the prose class
// (ADR-0020) that routes to neither the proof path nor the advisory panel.
func proseRecord() *record.Record {
	return &record.Record{
		// Kind "trap" (not "convention") is what retro-capture primarily emits and is
		// subject to the validated-trap guard requirement — the prose panel is its proof.
		SchemaVersion: 1, ID: "exp-0200", Kind: "trap", Status: "quarantined",
		Title:   "Prefer errors.Is over == for wrapped sentinel errors, long enough title",
		Symptom: &record.Symptom{Summary: "Comparing a wrapped error with == silently misses the sentinel."},
		Resolution: &record.Resolution{
			RootCause: "fmt.Errorf(\"%w\", err) wraps the sentinel, so == against the bare sentinel fails.",
			Fix:       "Use errors.Is(err, ErrSentinel) to match through the wrap chain.",
		},
		AppliesTo: []record.AppliesTo{{Ecosystem: "Go", Package: "errors"}},
		Provenance: record.Provenance{
			Source: record.Source{Author: "retro-capture"}, RecordedAt: "2026-06-18",
			Valid: record.Validity{From: "2026-06-18"},
		},
		Body: "A session captured this: comparing wrapped errors with == misses the sentinel; use errors.Is.",
		Path: "experience/2026/0200-errors-is.md",
	}
}

// prosePanel builds a two-member, distinct-family stub panel (gpt-oss + agy) with the
// given member decisions — the cross-family prose panel (ADR-0020), gemini-free.
func prosePanel(t *testing.T, v1, v2 judge.Verdict) judge.Judge {
	t.Helper()
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{Verdict: v1}},
		judge.PanelMember{Model: "agy-pro", Judge: &judge.StubJudge{Verdict: v2}},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	return panel
}

func TestPromote_ProsePanelApproves_Promotes(t *testing.T) {
	att := &stubAttestor{att: holdingAtt()}
	panel := prosePanel(t, judge.ApproveVerdict("gpt-oss:20b"), judge.ApproveVerdict("agy-pro"))
	proofJudge := &captureJudge{verdict: judge.ApproveVerdict("gpt-oss:20b")}
	p := newPromoter(t, att, proofJudge, promote.WithProsePanel(panel))
	rec := proseRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !out.Promoted {
		t.Fatalf("expected prose promotion, got reason %q", out.Reason)
	}
	if att.Calls != 0 {
		t.Fatalf("attestor must not be called on the prose path; calls=%d", att.Calls)
	}
	if proofJudge.last.Record != nil {
		t.Fatal("proof judge must not be consulted for prose records")
	}
	if rec.Status != "validated" {
		t.Fatalf("status = %q, want validated", rec.Status)
	}
	pr := rec.Provenance.Promotion
	if pr == nil || pr.AttestedAt != "" || len(pr.Panel) != 2 {
		t.Fatalf("promotion audit block wrong (no attestation, two panel verdicts): %+v", pr)
	}
	if pr.JudgeModel != "gpt-oss:20b+agy-pro" || pr.JudgeDecision != "approve" {
		t.Fatalf("top-level promotion fields wrong: %+v", pr)
	}
	if err := record.Validate(rec); err != nil {
		t.Fatalf("promoted prose must validate: %v", err)
	}
}

func TestPromote_ProsePanelRejects_StaysQuarantined(t *testing.T) {
	panel := prosePanel(t, judge.ApproveVerdict("gpt-oss:20b"), judge.Verdict{Decision: judge.Reject})
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{}, promote.WithProsePanel(panel))
	rec := proseRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if out.Promoted || rec.Status != "quarantined" {
		t.Fatal("one prose-panel dissent must not promote")
	}
}

func TestPromote_ProsePanelError_FailSafe(t *testing.T) {
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{Err: errors.New("down")}},
		judge.PanelMember{Model: "agy-pro", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("agy-pro")}},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{}, promote.WithProsePanel(panel))
	rec := proseRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("panel error is fail-safe, not a hard error: %v", err)
	}
	if out.Promoted || rec.Status != "quarantined" {
		t.Fatal("a prose-panel outage must leave the record quarantined")
	}
}

func TestPromote_ProseNoPanel_Skips(t *testing.T) {
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{})
	rec := proseRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if out.Promoted {
		t.Fatal("prose without a panel must not promote")
	}
	if out.Reason != "no prose panel configured — left for a human (ADR-0013 §5)" {
		t.Fatalf("reason = %q", out.Reason)
	}
}

// The mandatory content-screen (ADR-0020 §2c): a security-flagged prose record is never
// promoted, even with an approving panel.
func TestPromote_ProseSecurityFlagged_NotPromoted(t *testing.T) {
	panel := prosePanel(t, judge.ApproveVerdict("gpt-oss:20b"), judge.ApproveVerdict("agy-pro"))
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{}, promote.WithProsePanel(panel))
	rec := proseRecord()
	rec.Provenance.SecurityFlags = []string{"secret:aws-access-key"}

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if out.Promoted || rec.Status != "quarantined" {
		t.Fatal("a security-flagged prose record must never promote (mandatory content-screen)")
	}
}

// The prose panel receives Request.Prose=true (which selects ProsePanelSystemV1) and
// never Advisory — the routing flag the production prompt selection keys on.
func TestPromote_ProsePassesProseFlag(t *testing.T) {
	cj := &captureJudge{verdict: judge.ApproveVerdict("gpt-oss:20b")}
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: cj},
		judge.PanelMember{Model: "agy-pro", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("agy-pro")}},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{}, promote.WithProsePanel(panel))
	rec := proseRecord()

	if _, err := p.Promote(context.Background(), rec); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !cj.last.Prose {
		t.Fatal("the prose panel must receive Request.Prose=true (selects ProsePanelSystemV1)")
	}
	if cj.last.Advisory {
		t.Fatal("a prose request must not set Advisory")
	}
}

func TestPromotable_ProsePath(t *testing.T) {
	if ok, _ := promote.Promotable(proseRecord()); !ok {
		t.Fatal("a quarantined prose record must be promotable")
	}
	r := proseRecord()
	r.Status = "validated"
	if ok, _ := promote.Promotable(r); ok {
		t.Fatal("a non-quarantined prose record must not be promotable")
	}
}

func TestPromotable_AdvisoryAndProofPaths(t *testing.T) {
	if ok, _ := promote.Promotable(advisoryRecord()); !ok {
		t.Fatal("advisory quarantined record must be promotable")
	}
	if ok, _ := promote.Promotable(provableRecord()); !ok {
		t.Fatal("execution-provable record must be promotable")
	}
	if ok, reason := promote.Promotable(provableRecord()); ok {
		_ = reason
	}
	r := advisoryRecord()
	r.Status = "validated"
	if ok, _ := promote.Promotable(r); ok {
		t.Fatal("non-quarantined record must not be promotable")
	}
}

func TestPromote_AdvisoryPanelError_FailSafe(t *testing.T) {
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{Err: errors.New("down")}},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")}},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{}, promote.WithAdvisoryPanel(panel))
	rec := advisoryRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("panel error is fail-safe, not hard error: %v", err)
	}
	if out.Promoted || rec.Status != "quarantined" {
		t.Fatal("panel outage must leave advisory quarantined")
	}
}

func TestPromote_NonAdvisory_UnchangedProofPath(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	att := &stubAttestor{att: holdingAtt()}
	p := newPromoter(t, att, j)
	rec := provableRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !out.Promoted || att.Calls != 1 {
		t.Fatalf("non-advisory must use proof path: promoted=%v attestor_calls=%d", out.Promoted, att.Calls)
	}
}

// badClock yields a non-date so record.Validate fails on provenance.validated_at —
// the only knob that forces the post-mutation Validate to red without touching any
// product code, exercising each promotion path's guarded revert.
func badClock() promote.Option { return promote.WithClock(func() string { return "not-a-date" }) }

// The proof path mutates the record to validated, then validates; if the promoted
// record is invalid it MUST revert every mutated field and return a hard error so
// the caller never persists a half-promoted record (promote.go:242-245). A regression
// that dropped the revert (leaving status=validated with a bad validated_at) or
// returned Promoted:true on an invalid record would otherwise pass the whole suite.
func TestPromote_InvalidPromotedRecord_HardErrorAndReverts(t *testing.T) {
	j := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, j, badClock())
	rec := provableRecord()
	origStatus := rec.Status
	origValidatedAt := rec.Provenance.ValidatedAt
	origPromotion := rec.Provenance.Promotion

	out, err := p.Promote(context.Background(), rec)
	if err == nil {
		t.Fatal("an invalid promoted record must be a hard error, not a silent skip")
	}
	if !strings.Contains(err.Error(), "promoted record is invalid (not persisted)") {
		t.Fatalf("error must explain the not-persisted revert, got %q", err.Error())
	}
	if out.Promoted {
		t.Fatal("an invalid promoted record must not report Promoted")
	}
	if rec.Status != origStatus {
		t.Fatalf("status not reverted: %q, want %q", rec.Status, origStatus)
	}
	if rec.Provenance.ValidatedAt != origValidatedAt {
		t.Fatalf("validated_at not reverted: %v, want %v", rec.Provenance.ValidatedAt, origValidatedAt)
	}
	if rec.Provenance.Promotion != origPromotion {
		t.Fatalf("promotion block not reverted: %+v, want %+v", rec.Provenance.Promotion, origPromotion)
	}
}

// The advisory panel path has the same guarded revert (promote.go:281-284).
func TestPromote_AdvisoryInvalidPromotedRecord_HardErrorAndReverts(t *testing.T) {
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gpt-oss:20b")}},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")}},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{},
		promote.WithAdvisoryPanel(panel), badClock())
	rec := advisoryRecord()
	origStatus := rec.Status
	origValidatedAt := rec.Provenance.ValidatedAt
	origPromotion := rec.Provenance.Promotion

	out, err := p.Promote(context.Background(), rec)
	if err == nil {
		t.Fatal("an invalid promoted advisory must be a hard error, not a silent skip")
	}
	if !strings.Contains(err.Error(), "promoted record is invalid (not persisted)") {
		t.Fatalf("error must explain the not-persisted revert, got %q", err.Error())
	}
	if out.Promoted {
		t.Fatal("an invalid promoted advisory must not report Promoted")
	}
	if rec.Status != origStatus {
		t.Fatalf("status not reverted: %q, want %q", rec.Status, origStatus)
	}
	if rec.Provenance.ValidatedAt != origValidatedAt {
		t.Fatalf("validated_at not reverted: %v, want %v", rec.Provenance.ValidatedAt, origValidatedAt)
	}
	if rec.Provenance.Promotion != origPromotion {
		t.Fatalf("promotion block not reverted: %+v, want %+v", rec.Provenance.Promotion, origPromotion)
	}
}

// The prose panel path has the same guarded revert (promote.go:322-325).
func TestPromote_ProseInvalidPromotedRecord_HardErrorAndReverts(t *testing.T) {
	panel := prosePanel(t, judge.ApproveVerdict("gpt-oss:20b"), judge.ApproveVerdict("agy-pro"))
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{},
		promote.WithProsePanel(panel), badClock())
	rec := proseRecord()
	origStatus := rec.Status
	origValidatedAt := rec.Provenance.ValidatedAt
	origPromotion := rec.Provenance.Promotion

	out, err := p.Promote(context.Background(), rec)
	if err == nil {
		t.Fatal("an invalid promoted prose record must be a hard error, not a silent skip")
	}
	if !strings.Contains(err.Error(), "promoted record is invalid (not persisted)") {
		t.Fatalf("error must explain the not-persisted revert, got %q", err.Error())
	}
	if out.Promoted {
		t.Fatal("an invalid promoted prose record must not report Promoted")
	}
	if rec.Status != origStatus {
		t.Fatalf("status not reverted: %q, want %q", rec.Status, origStatus)
	}
	if rec.Provenance.ValidatedAt != origValidatedAt {
		t.Fatalf("validated_at not reverted: %v, want %v", rec.Provenance.ValidatedAt, origValidatedAt)
	}
	if rec.Provenance.Promotion != origPromotion {
		t.Fatalf("promotion block not reverted: %+v, want %+v", rec.Provenance.Promotion, origPromotion)
	}
}

func eolFinding(_ context.Context, r *record.Record) *doctor.Finding {
	return &doctor.Finding{RecordID: r.ID, Path: r.Path, Issue: "python 3.8 reached end-of-life 2024-10-01"}
}

// A born-stale advisory — one whose runtime is already EOL — must NOT be promoted
// even when the panel would unanimously approve: a validated EOL record trips the
// (validated-scoped) D2 staleness guard the instant it lands, which is what stuck
// ~36 validate PRs (#0071, the promote-side companion to #302). The promoter
// consults an injected staleness gate and holds the record, quarantined, BEFORE
// the costly panel call.
func TestPromote_AdvisoryEOLRuntime_HeldNotPromoted(t *testing.T) {
	m1 := &captureJudge{verdict: judge.ApproveVerdict("gpt-oss:20b")}
	m2 := &captureJudge{verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: m1},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: m2},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{},
		promote.WithAdvisoryPanel(panel), promote.WithStalenessGate(eolFinding))
	rec := advisoryRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if out.Promoted || rec.Status != "quarantined" {
		t.Fatalf("born-stale advisory must stay quarantined; promoted=%v status=%q", out.Promoted, rec.Status)
	}
	if m1.last.Record != nil || m2.last.Record != nil {
		t.Fatal("the panel must not be consulted once the staleness gate blocks (skip before the cost)")
	}
	if !strings.Contains(out.Reason, "end-of-life") {
		t.Fatalf("held reason should explain the EOL skip, got %q", out.Reason)
	}
}

// The gate must not over-block: a non-EOL advisory (gate returns nil) promotes
// exactly as before — the gate is a born-stale filter, not a new approver.
func TestPromote_AdvisoryNonEOLGate_Promotes(t *testing.T) {
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gpt-oss:20b")}},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")}},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	notStale := func(context.Context, *record.Record) *doctor.Finding { return nil }
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{},
		promote.WithAdvisoryPanel(panel), promote.WithStalenessGate(notStale))
	rec := advisoryRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !out.Promoted || rec.Status != "validated" {
		t.Fatalf("non-EOL advisory must promote; promoted=%v status=%q reason=%q", out.Promoted, rec.Status, out.Reason)
	}
}

// The prose path consults the SAME injected staleness gate as the advisory path
// (promote.go:299-303): a born-stale prose lesson is held, quarantined, BEFORE the
// costly cross-family panel — even when the panel would unanimously approve. The
// two paths share intent but are separate code; a bug wiring the gate into only one
// would otherwise pass (the advisory pair already covers its half).
func TestPromote_ProseEOLRuntime_HeldNotPromoted(t *testing.T) {
	m1 := &captureJudge{verdict: judge.ApproveVerdict("gpt-oss:20b")}
	m2 := &captureJudge{verdict: judge.ApproveVerdict("agy-pro")}
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: m1},
		judge.PanelMember{Model: "agy-pro", Judge: m2},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{},
		promote.WithProsePanel(panel), promote.WithStalenessGate(eolFinding))
	rec := proseRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if out.Promoted || rec.Status != "quarantined" {
		t.Fatalf("born-stale prose must stay quarantined; promoted=%v status=%q", out.Promoted, rec.Status)
	}
	if m1.last.Record != nil || m2.last.Record != nil {
		t.Fatal("the prose panel must not be consulted once the staleness gate blocks (skip before the cost)")
	}
	if !strings.Contains(out.Reason, "born-stale") {
		t.Fatalf("held reason should explain the born-stale skip, got %q", out.Reason)
	}
}

// The gate must not over-block the prose path: a non-stale prose record (gate
// returns nil) promotes exactly as before — the gate is a born-stale filter, not a
// new approver.
func TestPromote_ProseNonEOLGate_Promotes(t *testing.T) {
	panel := prosePanel(t, judge.ApproveVerdict("gpt-oss:20b"), judge.ApproveVerdict("agy-pro"))
	notStale := func(context.Context, *record.Record) *doctor.Finding { return nil }
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{},
		promote.WithProsePanel(panel), promote.WithStalenessGate(notStale))
	rec := proseRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !out.Promoted || rec.Status != "validated" {
		t.Fatalf("non-stale prose must promote; promoted=%v status=%q reason=%q", out.Promoted, rec.Status, out.Reason)
	}
}
