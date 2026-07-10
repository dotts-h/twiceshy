// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// The corpus importer (#0007) records where a license-clean fact came from, so
// the pack builder can mechanically keep commercial packs clean (ADR-0003 §4).
// Two additive `provenance` fields carry that: `source_license` (an SPDX id, or
// the sentinel "none (facts only)") and `source_url`. Both are OPTIONAL — old
// records still validate — and `source_license` is SPDX-shaped. These guard
// that contract across both validation layers (the Go validator and the
// normative JSON Schema).

// importerDraft is a minimal valid quarantined record in the shape the importer
// emits; tests mutate its provenance.
func importerDraft() *record.Record {
	return &record.Record{
		SchemaVersion: 1,
		ID:            "exp-9100",
		Kind:          "convention",
		Status:        "quarantined",
		Title:         "Imported license-clean version fact placeholder",
		AppliesTo:     []record.AppliesTo{{Ecosystem: "Go"}},
		Provenance: record.Provenance{
			Source:     record.Source{Author: "twiceshy-importer"},
			RecordedAt: "2026-06-18",
			Valid:      record.Validity{From: "2026-06-18"},
		},
		Body: "Distilled fact, authored in twiceshy's own words.",
		Path: "experience/2026/9100-imported-fact.md",
	}
}

func TestProvenance_SourceFieldsAreOptional(t *testing.T) {
	r := importerDraft() // neither source_license nor source_url set
	if err := record.Validate(r); err != nil {
		t.Fatalf("a record without source_license/source_url must validate: %v", err)
	}
}

func TestProvenance_SourceLicenseAcceptsSPDXAndSentinel(t *testing.T) {
	for _, lic := range []string{
		"MIT", "Apache-2.0", "CC-BY-4.0", "CC0-1.0", "GPL-3.0-only",
		record.SourceLicenseFactsOnly,
		record.SourceLicenseProjectAuthored,
	} {
		r := importerDraft()
		r.Provenance.SourceLicense = lic
		if lic != record.SourceLicenseProjectAuthored {
			r.Provenance.SourceURL = "https://github.com/advisories/GHSA-jfh8-c2jp-5v3q"
		}
		if err := record.Validate(r); err != nil {
			t.Errorf("source_license %q must be accepted: %v", lic, err)
		}
	}
}

func TestProvenance_ProjectAuthoredSentinelValidatesAndForbidsSourceURL(t *testing.T) {
	r := importerDraft()
	r.Provenance.SourceLicense = record.SourceLicenseProjectAuthored
	if err := record.Validate(r); err != nil {
		t.Fatalf("explicitly project-authored record must validate: %v", err)
	}
	out, err := record.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := loadRecordSchema(t).Validate(frontmatterValue(t, out)); err != nil {
		t.Fatalf("project-authored sentinel must satisfy the JSON Schema: %v", err)
	}
	r.Provenance.SourceURL = "https://example.com/not-project-authored"
	if err := record.Validate(r); err == nil {
		t.Fatal("project-authored rights evidence must not be combined with an external source URL")
	}
}

func TestProvenance_SourceLicenseRejectsMalformed(t *testing.T) {
	for _, lic := range []string{"not a license!", "GPL v3", "MIT OR Apache-2.0", "©2026"} {
		r := importerDraft()
		r.Provenance.SourceLicense = lic
		if err := record.Validate(r); err == nil {
			t.Errorf("malformed source_license %q must be rejected", lic)
		}
	}
}

// ADR-0011 §5: an authored-internal record carries the SourceLicenseAuthoredInternal
// sentinel and — by the authoring discipline — NO source_url (the fact was
// independently re-derived, not distilled from a URL). It must validate against
// both the Go validator and the normative JSON Schema.
func TestProvenance_AuthoredInternalSentinelValidates(t *testing.T) {
	r := importerDraft()
	r.Provenance.Source.Author = "claude"
	r.Provenance.SourceLicense = record.SourceLicenseAuthoredInternal // no source_url
	if err := record.Validate(r); err != nil {
		t.Fatalf("authored-internal record must validate: %v", err)
	}
	out, err := record.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	schema := loadRecordSchema(t)
	if err := schema.Validate(frontmatterValue(t, out)); err != nil {
		t.Errorf("authored-internal record must satisfy the schema: %v\n--- marshaled ---\n%s", err, out)
	}
}

// ADR-0011 §5: an authored-internal record is re-derived, not distilled from a
// URL, so a source_url on it is a discipline violation the validator must reject.
func TestProvenance_AuthoredInternalForbidsSourceURL(t *testing.T) {
	r := importerDraft()
	r.Provenance.Source.Author = "claude"
	r.Provenance.SourceLicense = record.SourceLicenseAuthoredInternal
	r.Provenance.SourceURL = "https://stackoverflow.com/q/123"
	if err := record.Validate(r); err == nil {
		t.Error("an authored-internal record with a source_url must be rejected (§5: re-derived, not distilled from a URL)")
	}
}

func TestProvenance_SourceURLRejectsNonHTTP(t *testing.T) {
	for _, u := range []string{"ftp://example.com/x", "notaurl", "javascript:alert(1)", "http://"} {
		r := importerDraft()
		r.Provenance.SourceURL = u
		if err := record.Validate(r); err == nil {
			t.Errorf("non-http(s) source_url %q must be rejected", u)
		}
	}
}

func TestProvenance_SourceFieldsRoundTripWhenSetOmitWhenEmpty(t *testing.T) {
	r := importerDraft()
	r.Provenance.SourceLicense = "CC-BY-4.0"
	r.Provenance.SourceURL = "https://github.com/advisories/GHSA-x"
	out, err := record.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(out), "source_license: CC-BY-4.0") ||
		!strings.Contains(string(out), "source_url: https://github.com/advisories/GHSA-x") {
		t.Fatalf("marshaled output missing source fields:\n%s", out)
	}
	got, err := record.Parse(r.Path, out)
	if err != nil {
		t.Fatalf("Parse round-trip: %v", err)
	}
	if got.Provenance.SourceLicense != "CC-BY-4.0" || got.Provenance.SourceURL != "https://github.com/advisories/GHSA-x" {
		t.Errorf("round-trip lost source fields: %+v", got.Provenance)
	}

	empty, err := record.Marshal(importerDraft())
	if err != nil {
		t.Fatalf("Marshal empty: %v", err)
	}
	if strings.Contains(string(empty), "source_license") || strings.Contains(string(empty), "source_url") {
		t.Errorf("empty source fields must be omitted, not null/empty-materialized:\n%s", empty)
	}
}

func TestProvenance_SourceFieldsSatisfyJSONSchema(t *testing.T) {
	schema := loadRecordSchema(t) // defined in schema_test.go (same package)
	r := importerDraft()
	r.Provenance.SourceLicense = "CC-BY-4.0"
	r.Provenance.SourceURL = "https://github.com/advisories/GHSA-x"
	if err := record.Validate(r); err != nil {
		t.Fatalf("fixture is not a valid record: %v", err)
	}
	out, err := record.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := schema.Validate(frontmatterValue(t, out)); err != nil {
		t.Errorf("record with source_license/source_url must satisfy the schema: %v\n--- marshaled ---\n%s", err, out)
	}
}

func TestProvenance_SourceAttributionRoundTripsAndSatisfiesSchema(t *testing.T) {
	r := importerDraft()
	r.Provenance.SourceLicense = "CC-BY-4.0"
	r.Provenance.SourceURL = "https://example.test/work"
	r.Provenance.SourceAttribution = &record.SourceAttribution{
		Creator: "Example Creator", Title: "Example Work",
		LicenseURL: "https://creativecommons.org/licenses/by/4.0/",
		Changes:    "Adapted into a concise record.", LicenseText: "Canonical legal code.",
	}
	out, err := record.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	got, err := record.Parse(r.Path, out)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provenance.SourceAttribution == nil || got.Provenance.SourceAttribution.Creator != "Example Creator" {
		t.Fatalf("round trip lost source attribution: %+v", got.Provenance.SourceAttribution)
	}
	if err := loadRecordSchema(t).Validate(frontmatterValue(t, out)); err != nil {
		t.Fatalf("source attribution must satisfy JSON Schema: %v", err)
	}
}
