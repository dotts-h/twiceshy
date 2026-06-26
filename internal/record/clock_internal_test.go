// SPDX-License-Identifier: AGPL-3.0-only

package record

import (
	"strings"
	"testing"
	"time"
)

// The validated-record past-window guard (validateProvenance, #0050) rejects a
// validated record whose valid.until lies before the validation instant. The boundary is
// deliberately a RAW UTC instant compared with `until.Before(now)` — byte-identical
// to the staleness doctor — and must NOT truncate `now` to start-of-day. If it did,
// a record with until == today (parsed as today 00:00 UTC) would compare
// today00:00.Before(today00:00) == false and wrongly VALIDATE, reintroducing the
// validated<->stale flip-flop the comment in record.go guards against.
//
// TestDesync_* (provenance_desync_test.go) only exercises far-past (2021) and
// far-future (2999) until dates — never the until==today / until==yesterday /
// until==tomorrow boundary, and never the injected instant. This pins exactly that
// boundary by calling the unexported validator (reachable only from package record).
func TestValidate_PastWindowBoundaryAgainstPinnedClock(t *testing.T) {
	fixed := time.Date(2026, 6, 23, 14, 30, 0, 0, time.UTC)

	// validatedRecord builds a minimal validated convention record (no guard
	// requirement fires for kind=convention) in the importer shape, mirroring the
	// validatedDraft() pattern used in the external _test package. Each case only
	// varies valid.until.
	validatedRecord := func(until string) *Record {
		v := "2026-06-18"
		r := &Record{
			SchemaVersion: 1,
			ID:            "exp-9100",
			Kind:          "convention",
			Status:        "validated",
			Title:         "Validated convention for the past-window boundary",
			AppliesTo:     []AppliesTo{{Ecosystem: "Go"}},
			Provenance: Provenance{
				Source:      Source{Author: "twiceshy-importer"},
				RecordedAt:  "2026-06-18",
				ValidatedAt: &v,
				Valid:       Validity{From: "2026-06-18"},
			},
			Body: "Distilled fact, authored in twiceshy's own words.",
			Path: "experience/2026/9100-imported-fact.md",
		}
		if until != "" {
			u := until
			r.Provenance.Valid.Until = &u
		}
		return r
	}

	// valid.until is parsed from YYYY-MM-DD via time.Parse, i.e. always
	// start-of-day UTC. Vary it by whole DAYS against the pinned clock — never by
	// nanosecond offsets — so the cases match how the field is actually parsed.
	cases := []struct {
		name       string
		until      string
		wantReject bool
	}{
		// Load-bearing: until == the pinned day. Parsed as 2026-06-23 00:00 UTC,
		// which IS before the pinned 14:30 instant → rejected. Proves the validator
		// does NOT truncate `now` to start-of-day; if it did this would wrongly pass.
		{"until is today (pinned day) is rejected", "2026-06-23", true},
		{"until is yesterday is rejected", "2026-06-22", true},
		{"until is tomorrow is accepted", "2026-06-24", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validatedRecord(tc.until)
			err := r.validate(fixed)
			if tc.wantReject {
				if err == nil {
					t.Fatalf("valid.until=%s with now=%s: want rejection, got nil", tc.until, fixed)
				}
				if !strings.Contains(err.Error(), "valid.until") {
					t.Errorf("rejection should name valid.until, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid.until=%s with now=%s: want accepted, got: %v", tc.until, fixed, err)
			}
		})
	}
}
