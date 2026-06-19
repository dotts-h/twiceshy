// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// An outcome report (#0031) is stored as a quarantined counter-record that
// disputes an existing record. The link to the disputed original is the
// additive, optional `provenance.disputes` field — an exp-id, shaped like
// `superseded_by`. #0032 follows it to re-run the original repro plus the
// counter. These guard the field across the Go validator and the round trip.

func TestProvenance_DisputesIsOptional(t *testing.T) {
	r := importerDraft() // no disputes set
	if err := record.Validate(r); err != nil {
		t.Fatalf("a record without disputes must validate: %v", err)
	}
}

func TestProvenance_DisputesAcceptsRecordID(t *testing.T) {
	r := importerDraft()
	disputed := "exp-0042"
	r.Provenance.Disputes = &disputed
	if err := record.Validate(r); err != nil {
		t.Errorf("a valid disputes id must be accepted: %v", err)
	}
}

func TestProvenance_DisputesRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"exp-1", "0042", "exp-abcd", "experience-0042", " exp-0042", "exp-0042 "} {
		r := importerDraft()
		b := bad
		r.Provenance.Disputes = &b
		if err := record.Validate(r); err == nil {
			t.Errorf("malformed disputes id %q must be rejected", bad)
		}
	}
}

func TestProvenance_DisputesRoundTrips(t *testing.T) {
	r := importerDraft()
	disputed := "exp-0042"
	r.Provenance.Disputes = &disputed

	out, err := record.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := record.Parse(r.Path, out)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if back.Provenance.Disputes == nil || *back.Provenance.Disputes != "exp-0042" {
		t.Fatalf("disputes did not round-trip: got %v", back.Provenance.Disputes)
	}
}
