// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

func stubNpm(responses map[string]string) ingest.NpmLiveOption {
	return ingest.WithNpmFetch(func(_ context.Context, pkg string) (io.ReadCloser, error) {
		body, ok := responses[pkg]
		if !ok {
			return nil, nil // 404 -> skip
		}
		return io.NopCloser(strings.NewReader(body)), nil
	})
}

func TestNpmLive_DeprecatedPackageYieldsFactsOnlyDraft(t *testing.T) {
	src := ingest.NewNpmLiveSource(
		ingest.WithNpmPackages([]string{"request", "express"}),
		stubNpm(map[string]string{
			"request": `{"version":"2.88.2","deprecated":"request has been deprecated, see https://example/issues/3142"}`,
			"express": `{"version":"4.18.2"}`, // not deprecated -> no draft
		}),
	)
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("want 1 draft (request only), got %d", len(drafts))
	}
	d := drafts[0]
	if d.Kind != "fix" || !strings.Contains(d.Title, "request") ||
		len(d.AppliesTo) != 1 || d.AppliesTo[0].Ecosystem != "npm" || d.AppliesTo[0].Package != "request" {
		t.Errorf("draft not scoped to npm/request: %+v", d)
	}
	if d.SourceLicense != record.SourceLicenseFactsOnly {
		t.Errorf("source_license = %q, want facts-only", d.SourceLicense)
	}
	if d.SourceURL != "https://www.npmjs.com/package/request" {
		t.Errorf("source_url = %q", d.SourceURL)
	}
	if got := d.Symptom.ErrorSignatures[0]; got != "npm warn deprecated request@2.88.2" {
		t.Errorf("signature = %q", got)
	}
	// Facts-only (ADR-0003 §4): the maintainer's verbatim deprecation message must
	// NOT be reproduced anywhere in the generated record.
	all := d.Title + d.Symptom.Summary + d.Resolution.RootCause + d.Resolution.Fix + d.Body
	if strings.Contains(all, "request has been deprecated, see") {
		t.Errorf("draft reproduced the verbatim npm deprecation message (facts-only violation):\n%s", all)
	}
}

// `deprecated: true` (boolean) counts; a 404 package is skipped, not fatal.
func TestNpmLive_BooleanDeprecatedAndMissingPackage(t *testing.T) {
	src := ingest.NewNpmLiveSource(
		ingest.WithNpmPackages([]string{"old", "gone"}),
		stubNpm(map[string]string{"old": `{"version":"1.0.0","deprecated":true}`}),
	)
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 1 || drafts[0].AppliesTo[0].Package != "old" {
		t.Fatalf("want 1 draft for 'old', got %d: %+v", len(drafts), drafts)
	}
}

// A malformed 200 body for one package must not zero out the rest (skip-junk).
func TestNpmLive_MalformedBodySkippedNotFatal(t *testing.T) {
	src := ingest.NewNpmLiveSource(
		ingest.WithNpmPackages([]string{"bad", "good"}),
		stubNpm(map[string]string{
			"bad":  `{not json`,
			"good": `{"version":"1.0.0","deprecated":"x"}`,
		}),
	)
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("a malformed body must not fail the batch: %v", err)
	}
	if len(drafts) != 1 || drafts[0].AppliesTo[0].Package != "good" {
		t.Errorf("malformed 'bad' skipped, 'good' kept; got %+v", drafts)
	}
}

// `deprecated:false` / absent must NOT yield a draft.
func TestNpmLive_NotDeprecatedNoDraft(t *testing.T) {
	src := ingest.NewNpmLiveSource(
		ingest.WithNpmPackages([]string{"a", "b"}),
		stubNpm(map[string]string{
			"a": `{"version":"1.0.0","deprecated":false}`,
			"b": `{"version":"2.0.0"}`,
		}),
	)
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 0 {
		t.Errorf("non-deprecated packages must yield no drafts, got %+v", drafts)
	}
}

func TestNpmLive_Name(t *testing.T) {
	if got := ingest.NewNpmLiveSource().Name(); got != "npm-deprecation" {
		t.Errorf("Name = %q, want npm-deprecation", got)
	}
}
