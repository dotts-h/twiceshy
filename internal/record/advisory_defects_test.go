// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

func strptr(s string) *string { return &s }

// advisoryRec builds a minimal advisory-class record (carries a GHSA id) with the
// given affected entries and fix text, for the deterministic-defect detector tests.
func advisoryRec(id, fix, sourceURL string, applies []record.AppliesTo) *record.Record {
	return &record.Record{
		Symptom:    &record.Symptom{ErrorSignatures: []string{id}},
		AppliesTo:  applies,
		Resolution: &record.Resolution{Fix: fix},
		Provenance: record.Provenance{SourceURL: sourceURL},
	}
}

func TestAdvisoryDefects_NullFixedContradiction(t *testing.T) {
	id := "GHSA-aaaa-bbbb-cccc"
	url := "https://github.com/x/y/security/advisories/" + id
	// All affected ranges have fixed: null, yet the fix text promises an upgrade
	// past the fixed version — the #0061 Defect-3 contradiction (exp-0061's class).
	rec := advisoryRec(id,
		"Upgrade affected packages past the fixed version; see "+url+".",
		url,
		[]record.AppliesTo{{Ecosystem: "Go", Package: "github.com/x/y", Versions: &record.VersionRange{Introduced: strptr("0"), Fixed: nil}}},
	)
	got := record.AdvisoryDefects(rec)
	if !hasFlagPrefix(got, "consistency:null-fixed") {
		t.Fatalf("expected null-fixed contradiction flag, got %v", got)
	}
}

func TestAdvisoryDefects_NullFixedNoContradictionWhenTextHonest(t *testing.T) {
	id := "GHSA-aaaa-bbbb-cccc"
	url := "https://github.com/x/y/security/advisories/" + id
	// fixed: null AND the honest "No fix is published yet" text → no contradiction.
	rec := advisoryRec(id,
		"No fix is published yet (the advisory lists no fixed version); see "+url+" for status.",
		url,
		[]record.AppliesTo{{Ecosystem: "Go", Package: "github.com/x/y", Versions: &record.VersionRange{Fixed: nil}}},
	)
	if got := record.AdvisoryDefects(rec); hasFlagPrefix(got, "consistency:null-fixed") {
		t.Fatalf("honest no-fix text must not be flagged, got %v", got)
	}
}

func TestAdvisoryDefects_CleanFixedVersionPasses(t *testing.T) {
	id := "GHSA-aaaa-bbbb-cccc"
	url := "https://github.com/x/y/security/advisories/" + id
	rec := advisoryRec(id,
		"Upgrade affected packages past the fixed version; see "+url+".",
		url,
		[]record.AppliesTo{{Ecosystem: "Go", Package: "github.com/x/y", Versions: &record.VersionRange{Fixed: strptr("1.2.3")}}},
	)
	if got := record.AdvisoryDefects(rec); len(got) != 0 {
		t.Fatalf("clean advisory must have no defects, got %v", got)
	}
}

func TestAdvisoryDefects_SourceURLIDMismatch(t *testing.T) {
	id := "GHSA-aaaa-bbbb-cccc"
	// source_url cites a DIFFERENT advisory id than the record carries (#0061 Defect 4).
	otherURL := "https://github.com/x/y/security/advisories/GHSA-zzzz-yyyy-xxxx"
	rec := advisoryRec(id,
		"Upgrade affected packages past the fixed version; see "+otherURL+".",
		otherURL,
		[]record.AppliesTo{{Ecosystem: "Go", Package: "github.com/x/y", Versions: &record.VersionRange{Fixed: strptr("1.2.3")}}},
	)
	got := record.AdvisoryDefects(rec)
	if !hasFlagPrefix(got, "consistency:source-url-id-mismatch") {
		t.Fatalf("expected source-url id-mismatch flag, got %v", got)
	}
}

func TestAdvisoryDefects_MatchingSourceURLPasses(t *testing.T) {
	id := "GHSA-aaaa-bbbb-cccc"
	url := "https://github.com/x/y/security/advisories/" + id
	rec := advisoryRec(id,
		"Upgrade affected packages past the fixed version; see "+url+".",
		url,
		[]record.AppliesTo{{Ecosystem: "Go", Package: "github.com/x/y", Versions: &record.VersionRange{Fixed: strptr("1.2.3")}}},
	)
	if got := record.AdvisoryDefects(rec); hasFlagPrefix(got, "consistency:source-url-id-mismatch") {
		t.Fatalf("matching source_url must not be flagged, got %v", got)
	}
}

func TestAdvisoryDefects_MalformedPackagePath(t *testing.T) {
	id := "GHSA-aaaa-bbbb-cccc"
	url := "https://github.com/x/y/security/advisories/" + id
	// package carries an https:// prefix — never a valid module coordinate (exp-0022).
	rec := advisoryRec(id,
		"Upgrade affected packages past the fixed version; see "+url+".",
		url,
		[]record.AppliesTo{{Ecosystem: "Go", Package: "https://github.com/x/y", Versions: &record.VersionRange{Fixed: strptr("1.2.3")}}},
	)
	got := record.AdvisoryDefects(rec)
	if !hasFlagPrefix(got, "consistency:malformed-package-path") {
		t.Fatalf("expected malformed-package-path flag, got %v", got)
	}
}

func TestAdvisoryDefects_KnownAuditedScopeAndGoModulePathDefects(t *testing.T) {
	tests := []struct {
		name, id, ecosystem, pkg, want string
	}{
		{"non-Go library mislabeled Go", "GHSA-22fx-6r9m-r8h9", "Go", "github.com/strukturag/libheif", "consistency:ecosystem-package-mismatch"},
		{"fabricated Prometheus v2 suffix", "GHSA-4v48-4q5m-8vx4", "Go", "github.com/prometheus/prometheus/v2", "consistency:go-major-version-path"},
		{"missing Traefik v2 suffix", "GHSA-7v4p-328v-8v5g", "Go", "github.com/traefik/traefik", "consistency:go-major-version-path"},
		{"mis-cased Ech0 path", "GHSA-fpw6-hrg5-q5x5", "Go", "github.com/lin-snow/Ech0", "consistency:go-module-path-case"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := advisoryRec(tc.id, "No fix is published yet.", "https://osv.dev/vulnerability/"+tc.id,
				[]record.AppliesTo{{Ecosystem: tc.ecosystem, Package: tc.pkg}})
			got := record.AdvisoryDefects(rec)
			if !hasFlagPrefix(got, tc.want) {
				t.Fatalf("expected %s for audited defect, got %v", tc.want, got)
			}
			if !hasFlagPrefix(record.AdvisoryBlockingDefects(rec), tc.want) {
				t.Fatalf("audited defect must block promotion, got %v", record.AdvisoryBlockingDefects(rec))
			}
		})
	}
}

func TestAdvisoryDefects_DoesNotGeneralizeAuditedModuleFacts(t *testing.T) {
	// Mixed case and v2+ release numbers are not intrinsically wrong in Go:
	// canonical modules such as Azure use uppercase, while projects such as
	// Prometheus publish v2 application releases from an unsuffixed module.
	tests := []*record.Record{
		advisoryRec("GHSA-aaaa-bbbb-cccc", "No fix is published yet.", "https://osv.dev/vulnerability/GHSA-aaaa-bbbb-cccc",
			[]record.AppliesTo{{Ecosystem: "Go", Package: "github.com/Azure/azure-sdk-for-go"}}),
		advisoryRec("GHSA-aaaa-bbbb-cccc", "No fix is published yet.", "https://osv.dev/vulnerability/GHSA-aaaa-bbbb-cccc",
			[]record.AppliesTo{{Ecosystem: "Go", Package: "github.com/example/project", Versions: &record.VersionRange{Fixed: strptr("2.3.4")}}}),
	}
	for _, rec := range tests {
		for _, flag := range record.AdvisoryDefects(rec) {
			if strings.HasPrefix(flag, "consistency:go-") || strings.HasPrefix(flag, "consistency:ecosystem-package") {
				t.Fatalf("ambiguous package evidence must not be auto-flagged: %v", record.AdvisoryDefects(rec))
			}
		}
	}
}

func TestAdvisoryDefects_NonAdvisoryReturnsNil(t *testing.T) {
	// A prose record (no vuln id) is out of scope for the advisory defect gate.
	rec := &record.Record{
		Symptom:    &record.Symptom{Summary: "wrapping errors with == misses the sentinel"},
		Resolution: &record.Resolution{Fix: "Upgrade affected packages past the fixed version."},
		AppliesTo:  []record.AppliesTo{{Ecosystem: "Go", Package: "https://nope", Versions: &record.VersionRange{Fixed: nil}}},
	}
	if got := record.AdvisoryDefects(rec); got != nil {
		t.Fatalf("non-advisory record must return nil, got %v", got)
	}
}

// consistency_flags (#0061) mirror security_flags: a quarantined record may carry
// them; a validated record may NOT (a flagged record cannot be promoted) — the
// deterministic rule-based gate independent of the LLM judge.
func TestConsistencyFlags_QuarantinedValidVsValidatedRejected(t *testing.T) {
	q := importerDraft() // from provenance_license_test.go (same package)
	q.Status = "quarantined"
	q.Provenance.ConsistencyFlags = []string{"consistency:null-fixed-fix-text"}
	if err := record.Validate(q); err != nil {
		t.Errorf("quarantined record with consistency_flags must validate: %v", err)
	}

	v := importerDraft()
	v.Status = "validated"
	vd := "2026-06-18"
	v.Provenance.ValidatedAt = &vd
	v.Provenance.ConsistencyFlags = []string{"consistency:null-fixed-fix-text"}
	err := record.Validate(v)
	if err == nil {
		t.Fatal("a validated record with consistency_flags must be rejected")
	}
	if !strings.Contains(err.Error(), "consistency_flags") {
		t.Errorf("error should mention consistency_flags, got: %v", err)
	}
}

// AdvisoryBlockingDefects hard-blocks every deterministic #0061 defect.
func TestAdvisoryBlockingDefects_NullFixedAndMalformedAreBlocking(t *testing.T) {
	id := "GHSA-aaaa-bbbb-cccc"
	url := "https://github.com/x/y/security/advisories/" + id
	nullFixed := advisoryRec(id, "Upgrade past the fixed version; see "+url+".", url,
		[]record.AppliesTo{{Ecosystem: "Go", Package: "github.com/x/y", Versions: &record.VersionRange{Fixed: nil}}})
	if !hasFlagPrefix(record.AdvisoryBlockingDefects(nullFixed), "consistency:null-fixed") {
		t.Fatalf("null-fixed must be blocking, got %v", record.AdvisoryBlockingDefects(nullFixed))
	}
	malformed := advisoryRec(id, "Upgrade past the fixed version; see "+url+".", url,
		[]record.AppliesTo{{Ecosystem: "Go", Package: "https://github.com/x/y", Versions: &record.VersionRange{Fixed: strptr("1.2.3")}}})
	if !hasFlagPrefix(record.AdvisoryBlockingDefects(malformed), "consistency:malformed-package-path") {
		t.Fatalf("malformed path must be blocking, got %v", record.AdvisoryBlockingDefects(malformed))
	}
}

func TestAdvisoryBlockingDefects_SourceURLMismatchBlocks(t *testing.T) {
	id := "GHSA-aaaa-bbbb-cccc"
	otherURL := "https://github.com/x/y/security/advisories/GHSA-zzzz-yyyy-xxxx"
	rec := advisoryRec(id, "Upgrade past the fixed version; see "+otherURL+".", otherURL,
		[]record.AppliesTo{{Ecosystem: "Go", Package: "github.com/x/y", Versions: &record.VersionRange{Fixed: strptr("1.2.3")}}})
	if !hasFlagPrefix(record.AdvisoryDefects(rec), "consistency:source-url-id-mismatch") {
		t.Fatal("source-url mismatch should be detected by AdvisoryDefects")
	}
	if !hasFlagPrefix(record.AdvisoryBlockingDefects(rec), "consistency:source-url-id-mismatch") {
		t.Fatalf("source-url mismatch must block promotion, got %v", record.AdvisoryBlockingDefects(rec))
	}
}

func TestConsistencyFlags_SourceURLMismatchRejectedAtValidate(t *testing.T) {
	v := importerDraft()
	v.Status = "validated"
	vd := "2026-06-18"
	v.Provenance.ValidatedAt = &vd
	v.Provenance.ConsistencyFlags = []string{"consistency:source-url-id-mismatch:GHSA-zzzz-yyyy-xxxx"}
	if err := record.Validate(v); err == nil {
		t.Fatal("a validated record with source-url mismatch must be rejected")
	}
}

func hasFlagPrefix(flags []string, prefix string) bool {
	for _, f := range flags {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}
