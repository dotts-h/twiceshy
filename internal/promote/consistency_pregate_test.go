// SPDX-License-Identifier: AGPL-3.0-only

package promote_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/promote"
)

// approvingAdvisoryPanel is a two-family panel that ALWAYS approves — so a held
// record proves the consistency pre-gate fired BEFORE the panel, not that the panel
// rejected it.
func approvingAdvisoryPanel(t *testing.T) judge.Judge {
	t.Helper()
	panel, err := judge.NewPanel(
		judge.PanelMember{Model: "gpt-oss:20b", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gpt-oss:20b")}},
		judge.PanelMember{Model: "gemini-2.5-pro", Judge: &judge.StubJudge{Verdict: judge.ApproveVerdict("gemini-2.5-pro")}},
	)
	if err != nil {
		t.Fatalf("NewPanel: %v", err)
	}
	return panel
}

// A LEGACY advisory carrying a null-fixed contradiction but NO stored
// consistency_flags (ingested before the ingest gate existed) must be HELD by the
// promote consistency pre-gate — even though the panel WOULD approve it. This is the
// gap the ingest gate alone can't close: the validate rule never fires because the
// stored flag is empty.
func TestPromote_ConsistencyPreGate_HoldsLegacyNullFixedContradiction(t *testing.T) {
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{}, promote.WithAdvisoryPanel(approvingAdvisoryPanel(t)))
	rec := advisoryRecord()
	rec.AppliesTo[0].Versions = nil       // fixed: null → "upgrade past the fixed version" is now a contradiction
	rec.Provenance.ConsistencyFlags = nil // legacy: no stored flag, so only a LIVE gate can catch it

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if out.Promoted {
		t.Fatal("a legacy null-fixed contradiction must be HELD, not promoted")
	}
	if rec.Status != "quarantined" {
		t.Fatalf("status = %q, want quarantined (held, fail-safe)", rec.Status)
	}
	if !strings.Contains(out.Reason, "consistency defect") {
		t.Fatalf("expected a consistency-defect hold reason, got %q", out.Reason)
	}
}

// A clean advisory (real fixed version, consistent source_url) promotes on panel
// approval — the pre-gate must not over-hold.
func TestPromote_ConsistencyPreGate_CleanAdvisoryPromotes(t *testing.T) {
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{}, promote.WithAdvisoryPanel(approvingAdvisoryPanel(t)))
	rec := advisoryRecord() // clean: carries a fixed version

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !out.Promoted {
		t.Fatalf("a clean advisory must promote, got held: %q", out.Reason)
	}
}

// A legacy advisory whose source_url names an id absent from the record's complete
// primary-id + alias set must be held before the panel. The detector is alias-aware,
// so this is concrete misdirection, not an ambiguous alias reference.
func TestPromote_ConsistencyPreGate_SourceURLMismatchHeld(t *testing.T) {
	p := newPromoter(t, &stubAttestor{att: holdingAtt()}, &judge.StubJudge{}, promote.WithAdvisoryPanel(approvingAdvisoryPanel(t)))
	rec := advisoryRecord() // clean fixed version (no null-fixed contradiction)
	// source_url cites a DIFFERENT GHSA id than the record's error_signatures.
	rec.Provenance.SourceURL = "https://github.com/x/y/security/advisories/GHSA-zzzz-yyyy-xxxx"

	out, err := p.Promote(context.Background(), rec)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if out.Promoted || !strings.Contains(out.Reason, "consistency defect") {
		t.Fatalf("a source-url mismatch must be held by the pre-gate, got %+v", out)
	}
}
