// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// The counter-evidence gate (#0032, ADR-0013 §3) adds the `disputed` status
// (independent non-reproducing reports accumulated past a threshold → escalate,
// reversible) and the `provenance.demotion` audit block (the counter-attestation
// + judge verdict that demoted a record). Both additive; guarded here across the
// Go validator and the round trip.

func TestStatus_DisputedAccepted(t *testing.T) {
	r := importerDraft()
	r.Status = "disputed"
	if err := record.Validate(r); err != nil {
		t.Fatalf("disputed must be a valid status: %v", err)
	}
}

func TestProvenance_DemotionAccepted(t *testing.T) {
	r := importerDraft()
	r.Status = "stale"
	r.Provenance.Demotion = &record.Demotion{
		AttestedAt:    "2026-06-19T00:00:00Z",
		JudgeModel:    "gemini-2.5-pro",
		JudgeDecision: "approve",
		Report:        "exp-0200",
	}
	if err := record.Validate(r); err != nil {
		t.Errorf("a well-formed demotion block must be accepted: %v", err)
	}
}

func TestProvenance_DemotionRejectsIncomplete(t *testing.T) {
	cases := map[string]record.Demotion{
		"missing attested_at": {JudgeModel: "g", JudgeDecision: "approve", Report: "exp-0200"},
		"missing judge_model": {AttestedAt: "2026-06-19T00:00:00Z", JudgeDecision: "approve", Report: "exp-0200"},
		"missing decision":    {AttestedAt: "2026-06-19T00:00:00Z", JudgeModel: "g", Report: "exp-0200"},
		"missing report":      {AttestedAt: "2026-06-19T00:00:00Z", JudgeModel: "g", JudgeDecision: "approve"},
		"malformed report id": {AttestedAt: "2026-06-19T00:00:00Z", JudgeModel: "g", JudgeDecision: "approve", Report: "not-an-id"},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			r := importerDraft()
			r.Status = "stale"
			dd := d
			r.Provenance.Demotion = &dd
			if err := record.Validate(r); err == nil {
				t.Errorf("%s: an incomplete demotion block must be rejected", name)
			}
		})
	}
}

func TestProvenance_DemotionRoundTrips(t *testing.T) {
	r := importerDraft()
	r.Status = "stale"
	r.Provenance.Demotion = &record.Demotion{
		AttestedAt: "2026-06-19T00:00:00Z", JudgeModel: "gemini-2.5-pro",
		JudgeDecision: "approve", Report: "exp-0200",
	}
	out, err := record.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := record.Parse(r.Path, out)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if back.Provenance.Demotion == nil || back.Provenance.Demotion.Report != "exp-0200" {
		t.Fatalf("demotion did not round-trip: %+v", back.Provenance.Demotion)
	}
}
