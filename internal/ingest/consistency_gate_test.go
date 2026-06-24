// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

// advisoryContradictionDraft is an advisory-class draft (carries a GHSA id) whose
// fix text promises an upgrade past a fixed version while every affected range has
// fixed: null — the #0061 null-fixed contradiction (exp-0061's class).
func advisoryContradictionDraft() ingest.Draft {
	id := "GHSA-aaaa-bbbb-cccc"
	url := "https://github.com/x/y/security/advisories/" + id
	return ingest.Draft{
		Kind:    "trap",
		Title:   "GHSA-aaaa-bbbb-cccc: vulnerability in github.com/x/y",
		Symptom: &record.Symptom{Summary: "known vulnerability in github.com/x/y", ErrorSignatures: []string{id}},
		AppliesTo: []record.AppliesTo{{
			Ecosystem: "Go", Package: "github.com/x/y",
			Versions: &record.VersionRange{Introduced: strptr("0"), Fixed: nil},
		}},
		Resolution:    &record.Resolution{RootCause: "documented in OSV", Fix: "Upgrade affected packages past the fixed version; see " + url + "."},
		Body:          "Imported OSV advisory for github.com/x/y; see the linked GHSA for details.",
		SourceLicense: "CC-BY-4.0",
		SourceURL:     url,
	}
}

// The consistency gate documents an advisory transcription defect in
// consistency_flags and keeps the record quarantined (never promotable) — the
// rule-based gate so the LLM judge is not the sole one.
func TestPrepare_ConsistencyGateFlagsNullFixedContradiction(t *testing.T) {
	ix := openIx(t)
	out, err := ingest.Prepare(context.Background(), ix, repo, advisoryContradictionDraft(), meta())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Record == nil {
		t.Fatal("expected a quarantined record, got nil")
	}
	if out.Record.Status != "quarantined" {
		t.Errorf("status = %q, want quarantined", out.Record.Status)
	}
	flags := out.Record.Provenance.ConsistencyFlags
	if len(flags) == 0 || !strings.HasPrefix(flags[0], "consistency:null-fixed") {
		t.Fatalf("expected a consistency:null-fixed flag, got %v", flags)
	}
}

// With RejectOnFlag the consistency gate refuses the draft outright (creates nothing).
func TestPrepare_ConsistencyGateRejectsWhenRejectOnFlag(t *testing.T) {
	ix := openIx(t)
	m := meta()
	m.RejectOnFlag = true
	_, err := ingest.Prepare(context.Background(), ix, repo, advisoryContradictionDraft(), m)
	if err == nil {
		t.Fatal("expected RejectOnFlag to refuse the contradictory draft, got nil error")
	}
	if !strings.Contains(err.Error(), "consistency gate") {
		t.Errorf("error = %v, want it to mention the consistency gate", err)
	}
}

// A clean advisory (real fixed version, matching source_url) passes the gate with no flags.
func TestPrepare_ConsistencyGatePassesCleanAdvisory(t *testing.T) {
	ix := openIx(t)
	d := advisoryContradictionDraft()
	d.AppliesTo[0].Versions.Fixed = strptr("1.2.3") // now the fix text is honest
	out, err := ingest.Prepare(context.Background(), ix, repo, d, meta())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Record == nil {
		t.Fatal("expected a record, got nil")
	}
	if len(out.Record.Provenance.ConsistencyFlags) != 0 {
		t.Errorf("clean advisory must have no consistency flags, got %v", out.Record.Provenance.ConsistencyFlags)
	}
}
