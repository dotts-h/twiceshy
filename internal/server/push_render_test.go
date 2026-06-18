// SPDX-License-Identifier: AGPL-3.0-only

package server_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/server"
)

func TestRenderTrapCardIncludesApplicabilityTrapEscape(t *testing.T) {
	rec := &record.Record{
		ID:     "exp-0042",
		Status: "validated",
		Title:  "Widget trap",
		Symptom: &record.Symptom{
			Summary: "widgets fail under load",
		},
		AppliesTo: []record.AppliesTo{
			{Ecosystem: "Go", Package: "example.com/widget"},
		},
		Resolution: &record.Resolution{
			Fix: "increase the widget buffer",
		},
	}
	got := server.RenderTrapCard(rec)
	for _, want := range []string{
		"exp-0042",
		"TRUST: validated",
		"Title: Widget trap",
		"Applies to: Go/example.com/widget",
		"The trap: widgets fail under load",
		"The escape: increase the widget buffer",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("trap card missing %q:\n%s", want, got)
		}
	}
}

func TestRenderPushContextEnvelopesCards(t *testing.T) {
	cards := []string{"card one", "card two"}
	got := server.RenderPushContext(cards)
	if got == "" {
		t.Fatal("non-empty cards must produce enveloped context")
	}
	for _, want := range []string{
		"TYPE: trap-cards",
		"TRUST: validated",
		"--- BEGIN EXPERIENCE DATA ---",
		"card one",
		"card two",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("push context missing %q:\n%s", want, got)
		}
	}
}

func TestRenderPushContextEmptyReturnsEmpty(t *testing.T) {
	if got := server.RenderPushContext(nil); got != "" {
		t.Errorf("empty cards must yield empty context, got %q", got)
	}
}
