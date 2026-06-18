// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

// The OSV/GHSA adapter (#0007) maps GitHub Advisory Database / OSV advisories
// to quarantined trap records: the GHSA/CVE id is the fingerprintable
// signature, applies_to mirrors the OSV affected ranges, and — because GHSA is
// CC-BY-4.0 — the records carry that license + a source_url (the
// include-with-attribution case the pack builder relies on).

func TestOSVSource_EmitsCCBYTraps(t *testing.T) {
	src := ingest.NewOSVSource()
	if src.Name() != "osv" {
		t.Fatalf("Name = %q, want osv", src.Name())
	}
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) < 2 {
		t.Fatalf("want >=2 advisory drafts, got %d", len(drafts))
	}
	for _, d := range drafts {
		if d.Kind != "trap" {
			t.Errorf("%q: kind = %q, want trap", d.Title, d.Kind)
		}
		if d.SourceLicense != "CC-BY-4.0" {
			t.Errorf("%q: source_license = %q, want CC-BY-4.0", d.Title, d.SourceLicense)
		}
		if d.SourceURL == "" {
			t.Errorf("%q: missing source_url (attribution required for CC-BY)", d.Title)
		}
		if d.Symptom == nil || len(d.Symptom.ErrorSignatures) == 0 {
			t.Errorf("%q: needs GHSA/CVE error signatures", d.Title)
		}
		if d.Resolution == nil || d.Resolution.RootCause == "" || d.Resolution.Fix == "" {
			t.Errorf("%q: a trap needs resolution.root_cause + fix", d.Title)
		}
		if len(d.AppliesTo) == 0 {
			t.Fatalf("%q: no applies_to mapped from affected ranges", d.Title)
		}
		for _, a := range d.AppliesTo {
			if a.Ecosystem == "" || a.Package == "" {
				t.Errorf("%q: applies_to needs ecosystem + package", d.Title)
			}
			if a.Versions == nil || a.Versions.Fixed == nil {
				t.Errorf("%q: applies_to needs a fixed version from the OSV range", d.Title)
			}
		}
	}
}

func TestOSVSource_DraftsPrepareIntoQuarantinedRecords(t *testing.T) {
	ix := openIx(t) // empty corpus — every advisory is Novel
	drafts, err := ingest.NewOSVSource().Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	for i, d := range drafts {
		m := ingest.Meta{ID: fmt.Sprintf("exp-%04d", i+1), Author: "twiceshy-importer", Now: "2026-06-18"}
		out, err := ingest.Prepare(context.Background(), ix, repo, d, m)
		if err != nil {
			t.Fatalf("Prepare(%q): %v", d.Title, err)
		}
		if out.Record == nil {
			t.Fatalf("draft %q unexpectedly deduped against empty corpus", d.Title)
		}
		if out.Record.Status != "quarantined" {
			t.Errorf("%q: status = %q, want quarantined", d.Title, out.Record.Status)
		}
		if out.Record.Provenance.SourceLicense != "CC-BY-4.0" || out.Record.Provenance.SourceURL == "" {
			t.Errorf("%q: CC-BY provenance not carried into record: %+v", d.Title, out.Record.Provenance)
		}
		if err := record.Validate(out.Record); err != nil {
			t.Errorf("imported record %q not schema-valid: %v", d.Title, err)
		}
	}
}
