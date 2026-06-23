// SPDX-License-Identifier: AGPL-3.0-only

package judge_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
)

func TestNewPanel_RejectsTooFewMembers(t *testing.T) {
	_, err := judge.NewPanel(judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{}})
	if err == nil {
		t.Fatal("panel with <2 members must error at construction")
	}
}

func TestNewPanel_RejectsEmptyModel(t *testing.T) {
	_, err := judge.NewPanel(
		judge.PanelMember{Model: "", Judge: &judge.StubJudge{}},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: &judge.StubJudge{}},
	)
	if err == nil {
		t.Fatal("empty member model must error")
	}
}

func TestNewPanel_RejectsNilJudge(t *testing.T) {
	_, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: nil},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: &judge.StubJudge{}},
	)
	if err == nil {
		t.Fatal("nil member judge must error")
	}
}

func TestNewPanel_RejectsSameFamily(t *testing.T) {
	_, err := judge.NewPanel(
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: &judge.StubJudge{}},
		judge.PanelMember{Model: "gemini-1.5-flash", Judge: &judge.StubJudge{}},
	)
	if err == nil {
		t.Fatal("two members of the same family must error at construction")
	}
}

func TestPanel_UnanimousApprove(t *testing.T) {
	a := &judge.StubJudge{Verdict: judge.ApproveVerdict("gpt-oss:20b")}
	b := &judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")}
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: a},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: b},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	v, err := panel.Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if !v.Approved() {
		t.Fatalf("unanimous panel must approve: %+v", v)
	}
	if v.Model != "gpt-oss:20b+gemini-2.5-pro" {
		t.Fatalf("Model = %q, want joined ids", v.Model)
	}
	pj, ok := panel.(interface{ PanelMembers() []judge.Verdict })
	if !ok {
		t.Fatal("panel must expose PanelMembers")
	}
	members := pj.PanelMembers()
	if len(members) != 2 || members[0].Model != "gpt-oss:20b" || members[1].Model != "gemini-2.5-pro" {
		t.Fatalf("panel must record both members: %+v", members)
	}
	if a.Calls != 1 || b.Calls != 1 {
		t.Fatalf("each member called once: a=%d b=%d", a.Calls, b.Calls)
	}
}

// The combined verdict's Approved() depends on the unioned Checks being rebuilt in
// canonical order (panel.go:95-98). This pins that contract: after unanimous approval
// the combined Checks are exactly the four canonical names, in order, all passing —
// and a non-canonical EXTRA check carried by one member is discarded by the rebuild.
func TestPanel_UnanimousApprove_CombinedChecksCanonical(t *testing.T) {
	a := &judge.StubJudge{Verdict: judge.ApproveVerdict("gpt-oss:20b")}
	// b approves, but carries an extra passing check beyond the canonical four.
	bv := judge.ApproveVerdict("gemini-2.5-pro")
	bv.Checks = append(bv.Checks, judge.Check{Name: "extra", Pass: true, Reason: "non-canonical"})
	b := &judge.StubJudge{Verdict: bv}
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: a},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: b},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	v, err := panel.Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if !v.Approved() {
		t.Fatalf("unanimous panel must approve even with a member's extra passing check: %+v", v)
	}
	if len(v.Checks) != len(judge.Checks) {
		t.Fatalf("combined Checks must hold exactly the %d canonical checks (extras dropped), got %d: %+v",
			len(judge.Checks), len(v.Checks), v.Checks)
	}
	for i, name := range judge.Checks {
		if v.Checks[i].Name != name {
			t.Fatalf("combined Checks[%d].Name = %q, want canonical %q (order must match judge.Checks)", i, v.Checks[i].Name, name)
		}
		if !v.Checks[i].Pass {
			t.Fatalf("combined Checks[%d] (%q) must pass after unanimous approval: %+v", i, name, v.Checks[i])
		}
	}
	for _, c := range v.Checks {
		if c.Name == "extra" {
			t.Fatalf("non-canonical extra check must be discarded by the canonical-order rebuild, found: %+v", v.Checks)
		}
	}
}

// When a seat's judge answers with a DIFFERENT model than its construction label
// (e.g. a Sonnet fallback behind a gemini-labelled frontier seat, #0086), the panel
// must record the model that actually answered — not silently relabel it.
func TestPanel_RecordsAnsweringModelNotLabel(t *testing.T) {
	local := &judge.StubJudge{Verdict: judge.ApproveVerdict("gpt-oss:20b")}
	// Seat labelled gemini, but the judge answers as claude-sonnet-4-6 (the fallback).
	frontier := &judge.StubJudge{Verdict: judge.ApproveVerdict("claude-sonnet-4-6")}
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: local},
		judge.PanelMember{Model: "gemini-2.5-flash", Judge: frontier},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	v, err := panel.Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if v.Model != "gpt-oss:20b+claude-sonnet-4-6" {
		t.Fatalf("combined Model = %q, want the answering models joined", v.Model)
	}
	members := panel.(interface{ PanelMembers() []judge.Verdict }).PanelMembers()
	if members[1].Model != "claude-sonnet-4-6" {
		t.Fatalf("frontier member must record the answering model, got %q", members[1].Model)
	}
}

func TestPanel_MemberErrorPropagates(t *testing.T) {
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gpt-oss:20b")}},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: &judge.StubJudge{Err: errors.New("endpoint down")}},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	_, err = panel.Judge(context.Background(), judge.Request{})
	if err == nil {
		t.Fatal("a member error must propagate (fail-safe)")
	}
}

func TestPanel_OneRejectNonApproving(t *testing.T) {
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gpt-oss:20b")}},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: &judge.StubJudge{Verdict: judge.Verdict{Decision: judge.Reject}}},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	v, err := panel.Judge(context.Background(), judge.Request{})
	if err != nil {
		t.Fatalf("a reject is not an error: %v", err)
	}
	if v.Approved() {
		t.Fatal("one dissent must yield a non-approving verdict")
	}
}
