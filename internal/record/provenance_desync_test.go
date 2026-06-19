// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// Validator desync guards (#0050, GO_LIVE_HARDENING_PLAN §C4). A manual reversal
// can leave a `validated` record carrying a past validity window or a lingering
// demotion block — the staleness doctor then re-flags it (validated↔stale
// flip-flop). validateProvenance must reject both at the source.

func validatedDraft() *record.Record {
	r := importerDraft() // provenance_license_test.go (same package)
	r.Status = "validated"
	v := "2026-06-18"
	r.Provenance.ValidatedAt = &v
	return r
}

func TestDesync_ValidatedWithPastValidUntil_Rejected(t *testing.T) {
	r := validatedDraft()
	// from before until before now — isolates the past-window guard from the
	// existing until<from ordering check.
	r.Provenance.Valid.From = "2020-01-01"
	past := "2021-01-01"
	r.Provenance.Valid.Until = &past
	err := record.Validate(r)
	if err == nil {
		t.Fatal("a validated record with a past valid.until must be rejected")
	}
	if !strings.Contains(err.Error(), "valid.until") {
		t.Errorf("error should mention valid.until, got: %v", err)
	}
}

func TestDesync_ValidatedWithFutureValidUntil_Valid(t *testing.T) {
	r := validatedDraft()
	future := "2999-01-01"
	r.Provenance.Valid.Until = &future
	if err := record.Validate(r); err != nil {
		t.Errorf("a validated record with a future validity window must validate: %v", err)
	}
}

func TestDesync_ValidatedWithDemotionBlock_Rejected(t *testing.T) {
	r := validatedDraft()
	r.Provenance.Demotion = &record.Demotion{
		AttestedAt:    "2026-06-19T00:00:00Z",
		JudgeModel:    "gemini-2.5-pro",
		JudgeDecision: "confirmed",
		Report:        "exp-9200",
	}
	err := record.Validate(r)
	if err == nil {
		t.Fatal("a validated record carrying a demotion block must be rejected")
	}
	if !strings.Contains(err.Error(), "demotion") {
		t.Errorf("error should mention demotion, got: %v", err)
	}
}

// Control: a properly-demoted stale record legitimately carries BOTH a past
// valid.until and a demotion block — the guards are scoped to `validated`.
func TestDesync_StaleWithPastUntilAndDemotion_Valid(t *testing.T) {
	r := validatedDraft()
	r.Status = "stale"
	r.Provenance.ValidatedAt = nil
	past := "2026-06-19"
	r.Provenance.Valid.Until = &past
	r.Provenance.Demotion = &record.Demotion{
		AttestedAt:    "2026-06-19T00:00:00Z",
		JudgeModel:    "gemini-2.5-pro",
		JudgeDecision: "confirmed",
		Report:        "exp-9200",
	}
	if err := record.Validate(r); err != nil {
		t.Errorf("a demoted stale record must validate: %v", err)
	}
}
