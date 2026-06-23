// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/guard"
	"github.com/dotts-h/twiceshy/internal/promote"
	"github.com/dotts-h/twiceshy/internal/record"
)

func TestPrintEffectPreview(t *testing.T) {
	actions := []promote.RecordAction{
		{ID: "exp-0100", Outcome: "promoted", FromStatus: "quarantined", ToStatus: "validated"},
		{ID: "exp-0101", Outcome: "held", FromStatus: "quarantined", ToStatus: "quarantined"},
	}
	var buf bytes.Buffer
	printEffectPreview(&buf, "promote", actions)
	out := buf.String()
	if !strings.Contains(out, "exp-0100: quarantined→validated") {
		t.Fatalf("missing promoted transition; got %q", out)
	}
	if strings.Contains(out, "exp-0101:") {
		t.Fatalf("unchanged held record must not appear as a transition line; got %q", out)
	}
	if !strings.Contains(out, "promote -effect: 1 of 2") {
		t.Fatalf("missing effect summary; got %q", out)
	}
	if !strings.Contains(out, "nothing written") {
		t.Fatalf("missing nothing-written footer; got %q", out)
	}
}

// printEffectPreview footer boundaries (main.go:963-972): the "1 of 2" case above
// leaves both extremes untested. (a) ALL records change exercises the path where the
// omit-unchanged branch never fires; (b) an EMPTY actions slice exercises the
// "0 of 0 … nothing written" footer a fully-ineligible/empty run produces. An
// off-by-one in the changed counter would slip past the single mixed-case test.
func TestPrintEffectPreview_AllChanged(t *testing.T) {
	actions := []promote.RecordAction{
		{ID: "exp-0100", Outcome: "promoted", FromStatus: "quarantined", ToStatus: "validated"},
		{ID: "exp-0101", Outcome: "promoted", FromStatus: "quarantined", ToStatus: "validated"},
	}
	var buf bytes.Buffer
	printEffectPreview(&buf, "promote", actions)
	out := buf.String()
	if !strings.Contains(out, "exp-0100: quarantined→validated") {
		t.Fatalf("missing exp-0100 transition; got %q", out)
	}
	if !strings.Contains(out, "exp-0101: quarantined→validated") {
		t.Fatalf("missing exp-0101 transition; got %q", out)
	}
	if !strings.Contains(out, "promote -effect: 2 of 2") {
		t.Fatalf("all-changed footer must read 2 of 2; got %q", out)
	}
}

func TestPrintEffectPreview_EmptyActions(t *testing.T) {
	var buf bytes.Buffer
	printEffectPreview(&buf, "promote", []promote.RecordAction{})
	out := buf.String()
	// Exact footer, em-dash included (main.go:971): a fully-ineligible/empty run.
	const wantFooter = "promote -effect: 0 of 0 record(s) would change status — nothing written"
	if !strings.Contains(out, wantFooter) {
		t.Fatalf("empty-run footer = %q, want it to contain %q", out, wantFooter)
	}
	if strings.Contains(out, "→") {
		t.Fatalf("empty actions must emit no transition line; got %q", out)
	}
}

func TestPromoteCorpus_NoOpPersist_ProducesTransitionActions(t *testing.T) {
	recs := []*record.Record{
		eligibleRec("exp-0100"), // promoted
		eligibleRec("exp-0101"), // held
	}
	fp := &fakePromoter{promote: map[string]bool{"exp-0100": true}}
	var persistCalls []string
	noopPersist := func(_ string, rec *record.Record) error {
		persistCalls = append(persistCalls, rec.ID)
		return nil
	}

	_, actions, err := promoteCorpus(context.Background(), ".", recs, fp, noopPersist, guard.Guardrails{}, nil, nil, &bytes.Buffer{}, "")
	if err != nil {
		t.Fatalf("promoteCorpus: %v", err)
	}
	if len(persistCalls) != 1 || persistCalls[0] != "exp-0100" {
		t.Fatalf("noop persist calls = %v, want [exp-0100] (promoted record still flows through persist)", persistCalls)
	}
	byID := actionByID(actions)
	a, ok := byID["exp-0100"]
	if !ok {
		t.Fatal("exp-0100 missing from actions")
	}
	if a.FromStatus != "quarantined" || a.ToStatus != "validated" {
		t.Fatalf("exp-0100 action = %+v, want quarantined→validated", a)
	}
	held, ok := byID["exp-0101"]
	if !ok {
		t.Fatal("exp-0101 missing from actions")
	}
	if held.FromStatus != held.ToStatus {
		t.Fatalf("held record should show no transition; got %+v", held)
	}
}
