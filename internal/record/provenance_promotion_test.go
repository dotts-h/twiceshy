// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// An autonomous promotion (#0029, ADR-0013) stamps its audit trail into the
// additive optional `provenance.promotion` block: the holding attestation it
// rode and the diverse judge's verdict, so every quarantined→validated flip
// carries — in the git commit — why it was allowed. These guard the field
// across the Go validator and the round trip.

func promotedDraft() *record.Record {
	r := importerDraft()
	r.Status = "validated"
	at := "2026-06-19"
	r.Provenance.ValidatedAt = &at
	return r
}

func TestProvenance_PromotionIsOptional(t *testing.T) {
	r := promotedDraft() // validated, no promotion block (e.g. a human promotion)
	if err := record.Validate(r); err != nil {
		t.Fatalf("a validated record without a promotion block must validate: %v", err)
	}
}

func TestProvenance_PromotionAccepted(t *testing.T) {
	r := promotedDraft()
	r.Provenance.Promotion = &record.Promotion{
		AttestedAt:      "2026-06-19T00:00:00Z",
		ReproducedUnder: []string{"go1.25"},
		JudgeModel:      "gemini-2.5-pro",
		JudgeDecision:   "approve",
	}
	if err := record.Validate(r); err != nil {
		t.Errorf("a well-formed promotion block must be accepted: %v", err)
	}
}

func TestProvenance_PromotionRejectsIncomplete(t *testing.T) {
	cases := map[string]record.Promotion{
		"neither attested_at nor panel": {JudgeModel: "gemini-2.5-pro", JudgeDecision: "approve"},
		"missing judge_model (proof)":   {AttestedAt: "2026-06-19T00:00:00Z", JudgeDecision: "approve"},
		"missing judge_decision (proof)": {
			AttestedAt: "2026-06-19T00:00:00Z", JudgeModel: "gemini-2.5-pro",
		},
		"panel member empty model": {
			JudgeModel: "gpt-oss:20b+gemini-2.5-pro", JudgeDecision: "approve",
			Panel: []record.PanelVerdict{{JudgeModel: "", JudgeDecision: "approve"}},
		},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			r := promotedDraft()
			pp := p
			r.Provenance.Promotion = &pp
			if err := record.Validate(r); err == nil {
				t.Errorf("%s: an incomplete promotion block must be rejected", name)
			}
		})
	}
}

func TestProvenance_PromotionPanelWithoutAttestationAccepted(t *testing.T) {
	r := promotedDraft()
	r.Provenance.Promotion = &record.Promotion{
		JudgeModel:    "gpt-oss:20b+gemini-2.5-pro",
		JudgeDecision: "approve",
		Panel: []record.PanelVerdict{
			{JudgeModel: "gpt-oss:20b", JudgeDecision: "approve"},
			{JudgeModel: "gemini-2.5-pro", JudgeDecision: "approve"},
		},
	}
	if err := record.Validate(r); err != nil {
		t.Fatalf("a panel promotion without attested_at must validate: %v", err)
	}
}

func TestProvenance_PromotionRoundTrips(t *testing.T) {
	r := promotedDraft()
	r.Provenance.Promotion = &record.Promotion{
		AttestedAt:      "2026-06-19T00:00:00Z",
		ReproducedUnder: []string{"go1.25"},
		JudgeModel:      "gemini-2.5-pro",
		JudgeDecision:   "approve",
	}
	out, err := record.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := record.Parse(r.Path, out)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if back.Provenance.Promotion == nil || back.Provenance.Promotion.JudgeModel != "gemini-2.5-pro" {
		t.Fatalf("promotion did not round-trip: %+v", back.Provenance.Promotion)
	}
	if len(back.Provenance.Promotion.ReproducedUnder) != 1 || back.Provenance.Promotion.ReproducedUnder[0] != "go1.25" {
		t.Fatalf("reproduced_under did not round-trip: %+v", back.Provenance.Promotion.ReproducedUnder)
	}
}
