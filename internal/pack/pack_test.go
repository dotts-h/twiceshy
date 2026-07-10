// SPDX-License-Identifier: AGPL-3.0-only

package pack_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/pack"
	"github.com/dotts-h/twiceshy/internal/record"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		license     string
		commercial  bool
		attribution bool
	}{
		{"", false, false},                                   // missing rights evidence — fail closed
		{record.SourceLicenseFactsOnly, true, false},         // distilled facts
		{record.SourceLicenseProjectAuthored, true, false},   // explicitly project-authored
		{record.SourceLicenseAuthoredInternal, false, false}, // §5-authored — internal-only, commercial-gated
		{"MIT", true, true},                                  // permissive copied material still carries notices
		{"Apache-2.0", true, true},                           //
		{"apache-2.0", true, true},                           // case-insensitive
		{"BSD-3-Clause", true, true},                         //
		{"BSD-2-Clause", true, true},                         //
		{"ISC", true, true},                                  //
		{"0BSD", true, true},                                 //
		{"CC0-1.0", true, true},                              // retain source/license notice in the pack ledger
		{"Unlicense", true, true},                            //
		{"CC-BY-4.0", true, true},                            // includable WITH attribution
		{"CC-BY-3.0", true, true},                            //
		{"cc-by-4.0", true, true},                            // case-insensitive
		{"CC-BY-SA-4.0", false, false},                       // share-alike — excluded
		{"cc-by-sa-3.0", false, false},                       //
		{"CC-BY-NC-4.0", false, false},                       // noncommercial — excluded (has cc-by- prefix)
		{"CC-BY-ND-4.0", false, false},                       // no-derivatives — excluded
		{"CC-BY-NC-SA-4.0", false, false},                    // noncommercial + share-alike — excluded
		{"GPL-3.0-only", false, false},                       // copyleft
		{"AGPL-3.0-only", false, false},                      //
		{"LGPL-2.1-only", false, false},                      //
		{"MPL-2.0", false, false},                            //
		{"EPL-2.0", false, false},                            //
		{"Foo-1.0", false, false},                            // unknown — fail closed
		{"proprietary", false, false},                        //
	}
	for _, tt := range tests {
		name := tt.license
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			e := pack.Classify(tt.license)
			if e.Commercial != tt.commercial || e.NeedsAttribution != tt.attribution {
				t.Errorf("Classify(%q) = {commercial:%v attribution:%v}, want {%v %v} (reason: %s)",
					tt.license, e.Commercial, e.NeedsAttribution, tt.commercial, tt.attribution, e.Reason)
			}
			if e.Reason == "" {
				t.Errorf("Classify(%q): empty reason", tt.license)
			}
			if e.Code == "" {
				t.Errorf("Classify(%q): empty reason code", tt.license)
			}
		})
	}
}

func TestClassifyReasonCodesAreStableAndRecordAware(t *testing.T) {
	if got := pack.Classify("").Code; got != pack.ReasonMissingEvidence {
		t.Fatalf("empty license code = %q, want %q", got, pack.ReasonMissingEvidence)
	}
	licensed := rec("exp-0001", "validated", "MIT", "")
	got := pack.ClassifyRecord(licensed)
	if got.Commercial || got.Code != pack.ReasonMissingSourceURL {
		t.Fatalf("licensed record without source URL = %+v", got)
	}
	licensed.Provenance.SourceURL = "https://example.test/upstream/commit/abc"
	got = pack.ClassifyRecord(licensed)
	if got.Commercial || got.Code != pack.ReasonMissingNoticeEvidence {
		t.Fatalf("licensed record without complete notice evidence = %+v", got)
	}
	licensed.Provenance.SourceAttribution = &record.SourceAttribution{
		CopyrightNotice: "Copyright 2026 Upstream Authors",
		LicenseText:     pack.ApprovedMITLicenseText,
	}
	attestRights(licensed)
	got = pack.ClassifyRecord(licensed)
	if !got.Commercial || got.Code != pack.ReasonLicensedNotice {
		t.Fatalf("licensed record with complete notice evidence = %+v", got)
	}
}

func TestClassifyRecordRequiresHumanReviewAndRejectsPlaceholderOrForgedAttestation(t *testing.T) {
	r := rec("exp-0001", "validated", record.SourceLicenseProjectAuthored, "")
	r.Provenance.RightsReview = nil
	if got := pack.ClassifyRecord(r); got.Commercial || got.Code != pack.ReasonMissingRightsReview {
		t.Fatalf("missing review attestation passed: %+v", got)
	}
	attestRights(r)
	r.Provenance.RightsReview.Reviewer = "TODO"
	r.Provenance.RightsReview.EvidenceSHA256 = pack.EvidenceDigest(r)
	if got := pack.ClassifyRecord(r); got.Commercial || got.Code != pack.ReasonMissingRightsReview {
		t.Fatalf("placeholder reviewer passed: %+v", got)
	}
	attestRights(r)
	r.Provenance.RightsReview.EvidenceSHA256 = "sha256:" + strings.Repeat("f", 64)
	if got := pack.ClassifyRecord(r); got.Commercial || got.Code != pack.ReasonRightsDigestMismatch {
		t.Fatalf("forged evidence digest passed: %+v", got)
	}
}

func TestEvidenceDigestBindsEveryDistributedRecordFieldAndProse(t *testing.T) {
	base := rec("exp-0001", "validated", record.SourceLicenseProjectAuthored, "")
	base.Title = "Original reviewed title"
	base.Body = "Original reviewed prose."
	base.Symptom = &record.Symptom{Summary: "Original symptom", ErrorSignatures: []string{"original error"}}
	attestRights(base)
	if got := pack.ClassifyRecord(base); !got.Commercial {
		t.Fatalf("reviewed baseline rejected: %+v", got)
	}

	mutations := map[string]func(*record.Record){
		"title":         func(r *record.Record) { r.Title = "Changed title" },
		"body":          func(r *record.Record) { r.Body = "Changed prose." },
		"signature":     func(r *record.Record) { r.Symptom.ErrorSignatures[0] = "changed error" },
		"status":        func(r *record.Record) { r.Status = "quarantined" },
		"source author": func(r *record.Record) { r.Provenance.Source.Author = "changed-author" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			clone := *base
			symptom := *base.Symptom
			symptom.ErrorSignatures = append([]string(nil), base.Symptom.ErrorSignatures...)
			clone.Symptom = &symptom
			mutate(&clone)
			if got := pack.ClassifyRecord(&clone); got.Commercial || got.Code != pack.ReasonRightsDigestMismatch {
				t.Fatalf("distributed record mutation did not invalidate review: %+v", got)
			}
		})
	}
}

func TestValidateCommercialArtifactsDetectsManifestAndNoticeDrift(t *testing.T) {
	recs := []*record.Record{
		withMITNotice(rec("exp-0001", "validated", "MIT", "https://example.test/upstream/commit/abc")),
		rec("exp-0002", "validated", record.SourceLicenseProjectAuthored, ""),
	}
	want := pack.BuildManifest(recs, true, false)
	notices := pack.NoticeDocument(want)
	materials := pack.MaterialFiles(recs, want)
	packLicense := []byte("Commercial pack terms\n")
	want.PackLicenseSHA256 = pack.LicenseDigest(packLicense)
	if errs := pack.ValidateCommercialArtifacts(recs, want, notices, packLicense, materials); len(errs) != 0 {
		t.Fatalf("canonical artifacts rejected: %v", errs)
	}

	badManifest := want
	badManifest.Attribution = nil
	if errs := pack.ValidateCommercialArtifacts(recs, badManifest, notices, packLicense, materials); len(errs) == 0 {
		t.Fatal("missing manifest notice entry must be rejected")
	}
	if errs := pack.ValidateCommercialArtifacts(recs, want, []byte("# incomplete\n"), packLicense, materials); len(errs) == 0 {
		t.Fatal("incomplete notice document must be rejected")
	}
	if errs := pack.ValidateCommercialArtifacts(recs, want, notices, nil, materials); len(errs) == 0 {
		t.Fatal("missing pack-level LICENSE terms must be rejected")
	}
	tampered := make(map[string][]byte, len(materials))
	for path, body := range materials {
		tampered[path] = append([]byte(nil), body...)
	}
	for path := range tampered {
		tampered[path] = []byte("forged material")
		break
	}
	if errs := pack.ValidateCommercialArtifacts(recs, want, notices, packLicense, tampered); len(errs) == 0 {
		t.Fatal("forged third-party material must be rejected")
	}
}

func TestClassifyRecordCCBYRequiresCompleteAttributionDetails(t *testing.T) {
	r := rec("exp-0001", "validated", "CC-BY-4.0", "https://example.test/work")
	r.Provenance.SourceAttribution = &record.SourceAttribution{
		Creator: "Example Creator", Title: "Example Work",
		LicenseURL:  "https://creativecommons.org/licenses/by/4.0/",
		LicenseText: "Creative Commons Attribution 4.0 International legal code",
	}
	if got := pack.ClassifyRecord(r); got.Commercial || got.Code != pack.ReasonMissingNoticeEvidence {
		t.Fatalf("missing change details must fail closed: %+v", got)
	}
	r.Provenance.SourceAttribution.Changes = "Condensed into an experience record; no source prose copied."
	attestRights(r)
	if got := pack.ClassifyRecord(r); got.Commercial || got.Code != pack.ReasonUnapprovedLicenseText {
		t.Fatalf("CC-BY without an approved canonical digest must remain fail-closed: %+v", got)
	}
}

func TestClassifyRecordApacheRequiresCopyrightNoticeAndLicenseText(t *testing.T) {
	r := rec("exp-0001", "validated", "Apache-2.0", "https://example.test/work")
	r.Provenance.SourceAttribution = &record.SourceAttribution{
		CopyrightNotice: "Copyright 2026 Example", LicenseText: "Apache License Version 2.0",
	}
	if got := pack.ClassifyRecord(r); got.Commercial || got.Code != pack.ReasonMissingNoticeEvidence {
		t.Fatalf("Apache record missing upstream NOTICE evidence must fail closed: %+v", got)
	}
	r.Provenance.SourceAttribution.Notice = "Upstream NOTICE material"
	attestRights(r)
	if got := pack.ClassifyRecord(r); got.Commercial || got.Code != pack.ReasonUnapprovedLicenseText {
		t.Fatalf("non-canonical Apache text must remain fail-closed: %+v", got)
	}
}

func TestNoticeDocumentBundlesLicenseAndAttributionMaterial(t *testing.T) {
	recs := []*record.Record{
		withMITNotice(rec("exp-0001", "validated", "MIT", "https://example.test/mit")),
	}
	doc := string(pack.NoticeDocument(pack.BuildManifest(recs, true, false)))
	for _, want := range []string{
		"THIRD_PARTY/exp-0001-COPYRIGHT.txt", "THIRD_PARTY/exp-0001-LICENSE.txt",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("notice document missing %q:\n%s", want, doc)
		}
	}
}

func TestNoticeDocumentEscapesHostileMetadataAndRawMaterialIsSeparate(t *testing.T) {
	r := withMITNotice(rec("exp-0001", "validated", "MIT", "https://example.test/mit"))
	r.Provenance.SourceAttribution.Title = "</script> [click](javascript:alert(1)) ```fence"
	r.Provenance.SourceAttribution.CopyrightNotice = "<b>raw copyright</b>"
	r.Provenance.SourceAttribution.Notice = "```html\n<img src=x onerror=alert(1)>\n```"
	attestRights(r)
	m := pack.BuildManifest([]*record.Record{r}, true, false)
	doc := string(pack.NoticeDocument(m))
	for _, hostile := range []string{"</script>", "javascript:alert", "```", "<b>"} {
		if strings.Contains(doc, hostile) {
			t.Fatalf("ATTRIBUTION contains unescaped/raw material %q:\n%s", hostile, doc)
		}
	}
	materials := pack.MaterialFiles([]*record.Record{r}, m)
	var joined string
	for _, body := range materials {
		joined += string(body)
	}
	if !strings.Contains(joined, "<img src=x onerror=alert(1)>") || !strings.Contains(joined, "Permission is hereby granted") {
		t.Fatalf("separate material files lost verbatim content: %q", joined)
	}
}

func withMITNotice(r *record.Record) *record.Record {
	r.Provenance.SourceAttribution = &record.SourceAttribution{
		Title: "Upstream MIT Work", CopyrightNotice: "Copyright 2026 Upstream Authors",
		LicenseText: pack.ApprovedMITLicenseText,
	}
	attestRights(r)
	return r
}

func withCCBYNotice(r *record.Record) *record.Record {
	r.Provenance.SourceAttribution = &record.SourceAttribution{
		Creator: "Upstream Creator", Title: "Upstream Work",
		LicenseURL: "https://creativecommons.org/licenses/by/4.0/",
		Changes:    "Condensed into an experience record.", LicenseText: "Creative Commons Attribution 4.0 legal code",
	}
	attestRights(r)
	return r
}

func attestRights(r *record.Record) {
	r.Provenance.RightsReview = &record.RightsReview{
		Reviewer: "Jane Rights Reviewer", ReviewedAt: "2026-07-10T12:00:00Z",
		SourceSHA256: "sha256:" + strings.Repeat("a", 64), Policy: pack.RightsPolicyV1,
	}
	r.Provenance.RightsReview.EvidenceSHA256 = pack.EvidenceDigest(r)
}

func rec(id, status, license, url string) *record.Record {
	r := &record.Record{
		ID:     id,
		Status: status,
		Provenance: record.Provenance{
			SourceLicense: license,
			SourceURL:     url,
		},
	}
	attestRights(r)
	return r
}

func TestBuildManifest_CommercialExcludesCopyleftAndAttributesCCBY(t *testing.T) {
	recs := []*record.Record{
		withMITNotice(rec("exp-0001", "validated", "MIT", "https://example.com/upstream")),
		withCCBYNotice(rec("exp-0002", "validated", "CC-BY-4.0", "https://github.com/advisories/GHSA-x")),
		rec("exp-0003", "validated", "CC-BY-SA-4.0", "https://example/sa"),
		rec("exp-0004", "validated", record.SourceLicenseProjectAuthored, ""),
		rec("exp-0005", "quarantined", "MIT", ""),
		rec("exp-0006", "validated", "", ""), // missing rights evidence
	}
	m := pack.BuildManifest(recs, true /*commercial*/, false /*includeQuarantined*/)

	got := map[string]bool{}
	for _, id := range m.Included {
		got[id] = true
	}
	// MIT and explicitly authored are in; CC-BY stays fail-closed until its
	// canonical license digest is approved; CC-BY-SA,
	// quarantined, and missing-rights records are out.
	for _, want := range []string{"exp-0001", "exp-0004"} {
		if !got[want] {
			t.Errorf("commercial pack should include %s; included=%v", want, m.Included)
		}
	}
	for _, no := range []string{"exp-0002", "exp-0003", "exp-0005", "exp-0006"} {
		if got[no] {
			t.Errorf("commercial pack must NOT include %s", no)
		}
	}
	// CC-BY-SA excluded for share-alike; quarantined excluded for status.
	reasons := map[string]string{}
	for _, e := range m.Excluded {
		reasons[e.ID] = e.Reason
	}
	if reasons["exp-0003"] == "" || reasons["exp-0005"] == "" || reasons["exp-0006"] == "" {
		t.Errorf("excluded records must carry reasons: %+v", m.Excluded)
	}
	if len(m.Attribution) != 1 || m.Attribution[0].ID != "exp-0001" {
		t.Errorf("attribution = %+v, want exp-0001", m.Attribution)
	}
}

func TestBuildManifest_CommercialCopiedMaterialNeedsSourceForNotice(t *testing.T) {
	m := pack.BuildManifest([]*record.Record{rec("exp-0001", "validated", "MIT", "")}, true, false)
	if len(m.Included) != 0 || len(m.Excluded) != 1 {
		t.Fatalf("copied licensed material without source URL must be excluded; manifest=%+v", m)
	}
}

func TestBuildManifest_CommercialMissingLicenseIsFailClosed(t *testing.T) {
	m := pack.BuildManifest([]*record.Record{rec("exp-0001", "validated", "", "")}, true, false)
	if len(m.Included) != 0 || len(m.Excluded) != 1 {
		t.Fatalf("missing source_license must be excluded; manifest=%+v", m)
	}
	if m.Excluded[0].Reason == "" {
		t.Fatal("missing-rights exclusion must explain the fail-closed verdict")
	}
}

func TestBuildManifest_NonCommercialIncludesAllValidated(t *testing.T) {
	recs := []*record.Record{
		rec("exp-0001", "validated", "GPL-3.0-only", ""), // copyleft ok in a non-commercial pack
		rec("exp-0002", "validated", "CC-BY-SA-4.0", ""), //
		rec("exp-0003", "quarantined", "MIT", ""),        // still excluded (not validated)
	}
	m := pack.BuildManifest(recs, false /*commercial*/, false)
	if len(m.Included) != 2 {
		t.Errorf("non-commercial pack should include both validated records; included=%v", m.Included)
	}
}

func TestBuildManifest_IncludeQuarantined(t *testing.T) {
	recs := []*record.Record{rec("exp-0001", "quarantined", "MIT", "")}
	if m := pack.BuildManifest(recs, false, false); len(m.Included) != 0 {
		t.Errorf("quarantined excluded by default; included=%v", m.Included)
	}
	if m := pack.BuildManifest(recs, false, true); len(m.Included) != 1 {
		t.Errorf("-include-quarantined should include it; included=%v", m.Included)
	}
}

// ADR-0011 §5: records authored from public-awareness *topics* (re-derived in our
// own words with original tests) are cleared for the INTERNAL corpus only; the
// commercial pack stays gated on a real legal review. The pack builder must keep
// them out of commercial packs (fail-closed) yet still ship them internally — the
// same build-time mechanism ADR-0003 §4 uses for copyleft.
func TestBuildManifest_CommercialExcludesAuthoredInternal(t *testing.T) {
	recs := []*record.Record{
		rec("exp-0001", "validated", record.SourceLicenseAuthoredInternal, ""),
		rec("exp-0002", "validated", record.SourceLicenseFactsOnly, ""), // distilled fact — commercial-safe
	}

	com := pack.BuildManifest(recs, true /*commercial*/, false)
	in := map[string]bool{}
	for _, id := range com.Included {
		in[id] = true
	}
	if in["exp-0001"] {
		t.Errorf("commercial pack must NOT include a §5-authored internal-only record; included=%v", com.Included)
	}
	if !in["exp-0002"] {
		t.Errorf("commercial pack should still include a distilled-facts record; included=%v", com.Included)
	}
	var reason string
	for _, e := range com.Excluded {
		if e.ID == "exp-0001" {
			reason = e.Reason
		}
	}
	if reason == "" {
		t.Errorf("the §5-authored exclusion must carry a reason; excluded=%+v", com.Excluded)
	}

	// The internal (non-commercial) pack still includes it — internal use is cleared.
	internal := pack.BuildManifest(recs, false /*commercial*/, false)
	inInternal := map[string]bool{}
	for _, id := range internal.Included {
		inInternal[id] = true
	}
	if !inInternal["exp-0001"] {
		t.Errorf("the internal pack should include the §5-authored record; included=%v", internal.Included)
	}
}
