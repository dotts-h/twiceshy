// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

// The Go deprecation adapter is the first importer source (#0007): it emits
// license-clean drafts (distilled facts, source_license = the facts-only
// sentinel) with a fingerprintable staticcheck SA1019 signature, and those
// drafts must Prepare into schema-valid quarantined records that carry the
// source provenance.

func TestGoSource_EmitsLicenseCleanDrafts(t *testing.T) {
	src := ingest.NewGoSource()
	if src.Name() != "go" {
		t.Fatalf("Name = %q, want go", src.Name())
	}
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) < 2 {
		t.Fatalf("want >=2 curated Go deprecation drafts, got %d", len(drafts))
	}
	for _, d := range drafts {
		if d.SourceLicense != record.SourceLicenseFactsOnly {
			t.Errorf("draft %q: source_license = %q, want the facts-only sentinel", d.Title, d.SourceLicense)
		}
		if d.SourceURL == "" {
			t.Errorf("draft %q: missing source_url", d.Title)
		}
		if d.Symptom == nil || len(d.Symptom.ErrorSignatures) == 0 {
			t.Errorf("draft %q: needs a fingerprintable error signature", d.Title)
		}
	}
}

func TestGoSource_DraftsPrepareIntoProvenancedRecords(t *testing.T) {
	ix := openIx(t) // empty corpus — every draft is Novel
	src := ingest.NewGoSource()
	drafts, err := src.Drafts(context.Background())
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
			t.Errorf("imported record must be quarantined, got %q", out.Record.Status)
		}
		if out.Record.Provenance.SourceLicense != record.SourceLicenseFactsOnly || out.Record.Provenance.SourceURL == "" {
			t.Errorf("source provenance not carried into record: %+v", out.Record.Provenance)
		}
		if err := record.Validate(out.Record); err != nil {
			t.Errorf("imported record %q not schema-valid: %v", d.Title, err)
		}
	}
}
