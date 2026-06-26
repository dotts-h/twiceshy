// SPDX-License-Identifier: AGPL-3.0-only

package promote_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
)

// TestPromote_RootCausePreGate_NoneHeld verifies that a record whose root_cause
// is absent/"None" is HELD (quarantined) before any attestor or panel call,
// and that the hold reason mentions root_cause (#0094).
func TestPromote_RootCausePreGate_NoneHeld(t *testing.T) {
	m1 := &captureJudge{verdict: judge.ApproveVerdict("gpt-oss:20b")}
	m2 := &captureJudge{verdict: judge.ApproveVerdict("agy-pro")}
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: m1},
		judge.PanelMember{Model: "agy-pro", Judge: m2},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	att := &stubAttestor{att: holdingAtt()}
	p := newPromoter(t, att, &judge.StubJudge{}, promote.WithProsePanel(panel))
	rec := proseRecord()
	rec.Resolution.RootCause = "None – a design convention to centralize parsing."

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if out.Promoted {
		t.Fatal("record with root_cause 'None ...' must be HELD, not promoted")
	}
	if rec.Status != "quarantined" {
		t.Fatalf("status = %q, want quarantined (held by root_cause pre-gate)", rec.Status)
	}
	if !strings.Contains(out.Reason, "root_cause") {
		t.Fatalf("held reason must mention root_cause, got %q", out.Reason)
	}
	// Attestor and panel must not be consulted — the hold is cheaper than any LLM call.
	if att.Calls != 0 {
		t.Fatalf("attestor must not be called on root_cause pre-gate hold; calls=%d", att.Calls)
	}
	if m1.last.Record != nil || m2.last.Record != nil {
		t.Fatal("prose panel must not be consulted once the root_cause pre-gate blocks")
	}
}

// TestPromote_RootCausePreGate_SubstantivePromotes verifies the pre-gate does not
// over-block: a record with a substantive root cause reaches the panel and promotes.
func TestPromote_RootCausePreGate_SubstantivePromotes(t *testing.T) {
	panel := prosePanel(t, judge.ApproveVerdict("gpt-oss:20b"), judge.ApproveVerdict("agy-pro"))
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{}, promote.WithProsePanel(panel))
	// proseRecord carries RootCause="fmt.Errorf(\"%w\", err) wraps..." — substantive.
	rec := proseRecord()

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !out.Promoted || rec.Status != "validated" {
		t.Fatalf("prose with substantive root cause must promote; promoted=%v status=%q reason=%q",
			out.Promoted, rec.Status, out.Reason)
	}
}

// TestHasSubstantiveRootCause is a focused unit table for the deterministic helper.
func TestHasSubstantiveRootCause(t *testing.T) {
	cases := []struct {
		rootCause string
		want      bool
	}{
		{"", false},
		{"None – a design convention to centralize parsing.", false},
		{"N/A", false},
		{"none.", false},
		{"none", false},
		{"none - short", false},
		{"The component only listened for resize", true},
		{"&& and || have equal precedence; the trailing || true swallowed the error", true},
		// "none" must match as a WORD, not a prefix — these are real root causes.
		{"Nonexistent database role lacked CREATE on the database", true},
		{"Nonempty buffer was reused across requests without a reset", true},
		{"Non-blocking socket returned EAGAIN and the read was treated as EOF", true},
	}
	for _, tc := range cases {
		rec := &record.Record{Resolution: &record.Resolution{RootCause: tc.rootCause}}
		got := promote.HasSubstantiveRootCause(rec)
		if got != tc.want {
			t.Errorf("HasSubstantiveRootCause(%q) = %v, want %v", tc.rootCause, got, tc.want)
		}
	}
	// nil Resolution must return false.
	if promote.HasSubstantiveRootCause(&record.Record{}) {
		t.Error("nil Resolution must return false")
	}
}
