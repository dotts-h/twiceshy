// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

// Distinctive OSV prose that must never appear in distilled drafts (license firewall).
const (
	osvLiveDistinctSummary = "UNIQUE_OSV_SUMMARY_PROSE_ZORBLAX_42"
	osvLiveDistinctDetails = "UNIQUE_OSV_DETAILS_PROSE_QUIBBLE_99"
)

// buildOSVLiveZip packs JSON advisory files into a zip ReadCloser for fetch injection.
func buildOSVLiveZip(t *testing.T, files map[string]any) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, rec := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %q: %v", name, err)
		}
		if err := json.NewEncoder(w).Encode(rec); err != nil {
			t.Fatalf("encode %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return io.NopCloser(bytes.NewReader(buf.Bytes()))
}

func osvLiveFixtureFiles() map[string]any {
	return map[string]any{
		"GO-2020-TEST1.json": map[string]any{
			"id":      "GO-2020-TEST1",
			"aliases": []string{"CVE-2020-8911", "GHSA-f5pg-7wfw-84q9"},
			"summary": osvLiveDistinctSummary,
			"details": osvLiveDistinctDetails,
			"affected": []map[string]any{
				{
					"package": map[string]string{"ecosystem": "Go", "name": "github.com/example/pkg"},
					"ranges": []map[string]any{
						{
							"type": "SEMVER",
							"events": []map[string]string{
								{"introduced": "0"},
								{"fixed": "1.2.3"},
							},
						},
					},
				},
			},
			"references": []map[string]string{
				{"type": "ADVISORY", "url": "https://github.com/advisories/GHSA-f5pg-7wfw-84q9"},
			},
		},
		"GO-2020-WITHDRAWN.json": map[string]any{
			"id":        "GO-2020-WITHDRAWN",
			"withdrawn": "2024-01-01T00:00:00Z",
			"summary":   "withdrawn advisory",
			"affected": []map[string]any{
				{
					"package": map[string]string{"ecosystem": "Go", "name": "github.com/withdrawn/pkg"},
					"ranges": []map[string]any{
						{"type": "SEMVER", "events": []map[string]string{{"introduced": "0"}}},
					},
				},
			},
		},
		"GHSA-npm-only.json": map[string]any{
			"id":      "GHSA-npm-only",
			"aliases": []string{"CVE-2019-10744"},
			"summary": "npm only advisory",
			"affected": []map[string]any{
				{
					"package": map[string]string{"ecosystem": "npm", "name": "lodash"},
					"ranges": []map[string]any{
						{
							"type": "SEMVER",
							"events": []map[string]string{
								{"introduced": "0"},
								{"fixed": "4.17.12"},
							},
						},
					},
				},
			},
		},
	}
}

func newOSVLiveTestSource(t *testing.T) ingest.Source {
	t.Helper()
	files := osvLiveFixtureFiles()
	return ingest.NewOSVLiveSource(ingest.WithOSVLiveFetch(func(_ context.Context) (io.ReadCloser, error) {
		return buildOSVLiveZip(t, files), nil
	}))
}

// Defect 3 (#0061), the largest transcription-defect class: an advisory with no
// fixed version (fixed:null) must NOT claim "upgrade past the fixed version" — that
// fix-text references a version that does not exist, a self-contradiction. Pairs
// with #0062 (the advisory judge now SEES the contradiction in the prompt).
// Disjoint affected intervals in one range must expand to one applies_to each,
// not collapse to first-introduced/last-fixed (which would falsely claim the gap
// between them is affected). A trailing open `introduced` closes as fixed:null.
func TestOSVLiveSource_DisjointRangeIntervalsExpand(t *testing.T) {
	files := map[string]any{
		"GO-2020-MULTI.json": map[string]any{
			"id":      "GO-2020-MULTI",
			"summary": "multi-interval advisory",
			"affected": []map[string]any{{
				"package": map[string]string{"ecosystem": "Go", "name": "github.com/example/multi"},
				"ranges": []map[string]any{{
					"type": "SEMVER",
					"events": []map[string]string{
						{"introduced": "1.0.0"}, {"fixed": "1.2.0"},
						{"introduced": "2.0.0"}, {"fixed": "2.3.0"},
					},
				}},
			}},
		},
		"GO-2020-TRAILOPEN.json": map[string]any{
			"id":      "GO-2020-TRAILOPEN",
			"summary": "trailing-open advisory",
			"affected": []map[string]any{{
				"package": map[string]string{"ecosystem": "Go", "name": "github.com/example/trail"},
				"ranges": []map[string]any{{
					"type": "SEMVER",
					"events": []map[string]string{
						{"introduced": "1.0.0"}, {"fixed": "1.2.0"}, {"introduced": "2.0.0"},
					},
				}},
			}},
		},
	}
	src := ingest.NewOSVLiveSource(ingest.WithOSVLiveFetch(func(_ context.Context) (io.ReadCloser, error) {
		return buildOSVLiveZip(t, files), nil
	}))
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}

	intervals := func(title string) map[string]string {
		t.Helper()
		for i := range drafts {
			if !strings.Contains(drafts[i].Title, title) {
				continue
			}
			got := map[string]string{}
			for _, a := range drafts[i].AppliesTo {
				intro, fixed := "", ""
				if a.Versions != nil {
					if a.Versions.Introduced != nil {
						intro = *a.Versions.Introduced
					}
					if a.Versions.Fixed != nil {
						fixed = *a.Versions.Fixed
					}
				}
				got[intro] = fixed
			}
			return got
		}
		t.Fatalf("no draft %q", title)
		return nil
	}

	multi := intervals("GO-2020-MULTI")
	if len(multi) != 2 || multi["1.0.0"] != "1.2.0" || multi["2.0.0"] != "2.3.0" {
		t.Errorf("disjoint intervals = %v, want 1.0.0->1.2.0 and 2.0.0->2.3.0 (not collapsed to 1.0.0->2.3.0)", multi)
	}
	trail := intervals("GO-2020-TRAILOPEN")
	if len(trail) != 2 || trail["1.0.0"] != "1.2.0" || trail["2.0.0"] != "" {
		t.Errorf("trailing-open intervals = %v, want 1.0.0->1.2.0 and 2.0.0->open", trail)
	}
}

// One affected package with TWO SEMVER ranges yields two applies_to, which forces
// osvLiveBody through its multi-fragment join: each package fragment is separated by
// exactly one "; " (the i>0 branch), and an introduced-only second range renders
// "(introduced X)" with no ", fixed" suffix. The body is rendered prose, so a wrong
// join or a stray ", fixed" is exactly the transcription-defect class this importer
// guards — yet every other fixture has a single range, leaving the join uncovered.
func TestOSVLiveSource_BodyJoinsMultipleRanges(t *testing.T) {
	files := map[string]any{
		"GO-2020-TWORANGE.json": map[string]any{
			"id":      "GO-2020-TWORANGE",
			"summary": "two-range advisory",
			"affected": []map[string]any{{
				"package": map[string]string{"ecosystem": "Go", "name": "github.com/example/pkg"},
				"ranges": []map[string]any{
					{"type": "SEMVER", "events": []map[string]string{{"introduced": "0"}, {"fixed": "1.0.0"}}},
					{"type": "SEMVER", "events": []map[string]string{{"introduced": "2.0.0"}}}, // introduced-only → fixed:null
				},
			}},
		},
	}
	src := ingest.NewOSVLiveSource(ingest.WithOSVLiveFetch(func(_ context.Context) (io.ReadCloser, error) {
		return buildOSVLiveZip(t, files), nil
	}))
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("want 1 draft, got %d", len(drafts))
	}
	d := drafts[0]
	if len(d.AppliesTo) != 2 {
		t.Fatalf("applies_to len = %d, want 2 (one per range)", len(d.AppliesTo))
	}
	// Exactly one "; " separator joins the two package fragments (the i>0 branch).
	if n := strings.Count(d.Body, "; "); n != 1 {
		t.Errorf("body must join two fragments with exactly one \"; \"; got %d in %q", n, d.Body)
	}
	// First fragment carries introduced+fixed; second is introduced-only with no fixed.
	if !strings.Contains(d.Body, "(introduced 0, fixed 1.0.0)") {
		t.Errorf("body %q missing the introduced+fixed fragment", d.Body)
	}
	if !strings.Contains(d.Body, "(introduced 2.0.0)") {
		t.Errorf("body %q missing the introduced-only fragment", d.Body)
	}
	if strings.Contains(d.Body, "(introduced 2.0.0, fixed") {
		t.Errorf("introduced-only range must not render a \", fixed\" suffix: %q", d.Body)
	}
}

func TestOSVLiveSource_NoFixedVersionFixText(t *testing.T) {
	files := map[string]any{
		"GO-2020-NOFIX.json": map[string]any{
			"id":      "GO-2020-NOFIX",
			"summary": "unfixed advisory",
			"affected": []map[string]any{{
				"package": map[string]string{"ecosystem": "Go", "name": "github.com/example/unfixed"},
				"ranges": []map[string]any{{
					"type":   "SEMVER",
					"events": []map[string]string{{"introduced": "0"}}, // no fixed event → fixed:null
				}},
			}},
		},
		"GO-2020-FIXED.json": map[string]any{
			"id":      "GO-2020-FIXED",
			"summary": "fixed advisory",
			"affected": []map[string]any{{
				"package": map[string]string{"ecosystem": "Go", "name": "github.com/example/fixed"},
				"ranges": []map[string]any{{
					"type":   "SEMVER",
					"events": []map[string]string{{"introduced": "0"}, {"fixed": "2.0.0"}},
				}},
			}},
		},
	}
	src := ingest.NewOSVLiveSource(ingest.WithOSVLiveFetch(func(_ context.Context) (io.ReadCloser, error) {
		return buildOSVLiveZip(t, files), nil
	}))
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	fix := map[string]string{}
	for _, d := range drafts {
		if d.Resolution == nil {
			t.Fatalf("draft %q has no resolution", d.Title)
		}
		switch {
		case strings.Contains(d.Title, "GO-2020-NOFIX"):
			fix["nofix"] = d.Resolution.Fix
		case strings.Contains(d.Title, "GO-2020-FIXED"):
			fix["fixed"] = d.Resolution.Fix
		}
	}
	if strings.Contains(fix["nofix"], "past the fixed version") {
		t.Errorf("fixed:null advisory must NOT claim a fixed version exists; got %q", fix["nofix"])
	}
	if !strings.Contains(strings.ToLower(fix["nofix"]), "no fix") {
		t.Errorf("fixed:null advisory should say no fix is published; got %q", fix["nofix"])
	}
	if !strings.Contains(fix["fixed"], "past the fixed version") {
		t.Errorf("a fixed advisory should still advise upgrading past the fix; got %q", fix["fixed"])
	}
}

func symptomEqual(a, b *record.Symptom) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Summary == b.Summary && fmt.Sprint(a.ErrorSignatures) == fmt.Sprint(b.ErrorSignatures)
}

func resolutionEqual(a, b *record.Resolution) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.RootCause == b.RootCause && a.Fix == b.Fix
}

func strPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func versionRangeEqual(a, b *record.VersionRange) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return strPtrEqual(a.Introduced, b.Introduced) && strPtrEqual(a.Fixed, b.Fixed)
}

func draftsEqual(a, b []ingest.Draft) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		da, db := a[i], b[i]
		if da.Kind != db.Kind || da.Title != db.Title || da.Body != db.Body ||
			da.SourceLicense != db.SourceLicense || da.SourceURL != db.SourceURL {
			return false
		}
		if !symptomEqual(da.Symptom, db.Symptom) {
			return false
		}
		if !resolutionEqual(da.Resolution, db.Resolution) {
			return false
		}
		if len(da.AppliesTo) != len(db.AppliesTo) {
			return false
		}
		for j := range da.AppliesTo {
			aa, ab := da.AppliesTo[j], db.AppliesTo[j]
			if aa.Ecosystem != ab.Ecosystem || aa.Package != ab.Package {
				return false
			}
			if !versionRangeEqual(aa.Versions, ab.Versions) {
				return false
			}
		}
	}
	return true
}

func TestOSVLiveSource_Name(t *testing.T) {
	if newOSVLiveTestSource(t).Name() != "osv-live" {
		t.Fatal(`Name() must return "osv-live"`)
	}
}

func TestOSVLiveSource_DeterministicMapping(t *testing.T) {
	src := newOSVLiveTestSource(t)
	ctx := context.Background()

	first, err := src.Drafts(ctx)
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	second, err := src.Drafts(ctx)
	if err != nil {
		t.Fatalf("Drafts (repeat): %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("want 1 Go draft (withdrawn + npm skipped), got %d", len(first))
	}
	if !draftsEqual(first, second) {
		t.Fatalf("re-running Drafts must yield identical output:\nfirst=%+v\nsecond=%+v", first, second)
	}

	d := first[0]
	if d.Kind != "trap" {
		t.Errorf("kind = %q, want trap", d.Kind)
	}
	if d.SourceLicense != "CC-BY-4.0" {
		t.Errorf("source_license = %q, want CC-BY-4.0", d.SourceLicense)
	}
	wantURL := "https://github.com/advisories/GHSA-f5pg-7wfw-84q9"
	if d.SourceURL != wantURL {
		t.Errorf("source_url = %q, want %q", d.SourceURL, wantURL)
	}
	if d.Symptom == nil {
		t.Fatal("missing symptom")
	}
	wantSigs := []string{"GO-2020-TEST1", "CVE-2020-8911", "GHSA-f5pg-7wfw-84q9"}
	if fmt.Sprint(d.Symptom.ErrorSignatures) != fmt.Sprint(wantSigs) {
		t.Errorf("error_signatures = %v, want %v", d.Symptom.ErrorSignatures, wantSigs)
	}
	wantSummary := "GO-2020-TEST1 (CVE-2020-8911, GHSA-f5pg-7wfw-84q9): known vulnerability in github.com/example/pkg"
	if d.Symptom.Summary != wantSummary {
		t.Errorf("symptom.summary = %q, want %q", d.Symptom.Summary, wantSummary)
	}
	if len(d.AppliesTo) != 1 {
		t.Fatalf("applies_to len = %d, want 1", len(d.AppliesTo))
	}
	a := d.AppliesTo[0]
	if a.Ecosystem != "Go" || a.Package != "github.com/example/pkg" {
		t.Errorf("applies_to = %+v, want Go/github.com/example/pkg", a)
	}
	if a.Versions == nil || a.Versions.Introduced == nil || *a.Versions.Introduced != "0" {
		t.Errorf("versions.introduced = %v, want 0", a.Versions)
	}
	if a.Versions.Fixed == nil || *a.Versions.Fixed != "1.2.3" {
		t.Errorf("versions.fixed = %v, want 1.2.3", a.Versions)
	}
}

func TestOSVLiveSource_LicenseFirewall(t *testing.T) {
	drafts, err := newOSVLiveTestSource(t).Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("want 1 draft, got %d", len(drafts))
	}
	d := drafts[0]
	var blob strings.Builder
	blob.WriteString(d.Title)
	blob.WriteString(d.Body)
	if d.Symptom != nil {
		blob.WriteString(d.Symptom.Summary)
	}
	if d.Resolution != nil {
		blob.WriteString(d.Resolution.RootCause)
		blob.WriteString(d.Resolution.Fix)
	}
	text := blob.String()
	if strings.Contains(text, osvLiveDistinctSummary) {
		t.Errorf("OSV summary prose leaked into draft: %q", osvLiveDistinctSummary)
	}
	if strings.Contains(text, osvLiveDistinctDetails) {
		t.Errorf("OSV details prose leaked into draft: %q", osvLiveDistinctDetails)
	}
}

func TestOSVLiveSource_PrepareQuarantinesAndDedups(t *testing.T) {
	src := newOSVLiveTestSource(t)
	ctx := context.Background()
	drafts, err := src.Drafts(ctx)
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("want 1 draft, got %d", len(drafts))
	}
	d := drafts[0]

	ix := openIx(t)
	meta := ingest.Meta{
		ID:                 "exp-0001",
		Author:             "twiceshy-importer",
		Now:                "2026-06-18",
		IncludeQuarantined: true,
	}
	out, err := ingest.Prepare(ctx, ix, repo, d, meta)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Novelty != index.NoveltyNovel {
		t.Fatalf("first Prepare want Novel, got %q", out.Novelty)
	}
	if out.Record == nil || out.Record.Status != "quarantined" {
		t.Fatalf("first Prepare must quarantine, got record=%v", out.Record)
	}
	if out.Record.Provenance.SourceLicense != "CC-BY-4.0" || out.Record.Provenance.SourceURL == "" {
		t.Errorf("CC-BY provenance not carried: %+v", out.Record.Provenance)
	}
	if err := record.Validate(out.Record); err != nil {
		t.Errorf("prepared record not schema-valid: %v", err)
	}

	// Rebuild index with the quarantined record — second Prepare must dedup.
	if err := ix.Rebuild(ctx, []*record.Record{out.Record}, repo); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	meta.ID = "exp-0002"
	out2, err := ingest.Prepare(ctx, ix, repo, d, meta)
	if err != nil {
		t.Fatalf("Prepare (second): %v", err)
	}
	if out2.Novelty == index.NoveltyNovel {
		t.Fatalf("second Prepare must dedup (Known or Similar), got Novel")
	}
	if out2.Record != nil {
		t.Errorf("deduped Prepare must not create another record, got %+v", out2.Record)
	}
}

func TestOSVLiveSource_SkipsWithdrawnAndNonGo(t *testing.T) {
	drafts, err := newOSVLiveTestSource(t).Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	for _, d := range drafts {
		if d.Symptom != nil {
			for _, sig := range d.Symptom.ErrorSignatures {
				if sig == "GO-2020-WITHDRAWN" || sig == "GHSA-npm-only" {
					t.Errorf("skipped advisory %q appeared in drafts", sig)
				}
			}
		}
	}
}

// WithEcosystem lets one importer cover a whole stack: npm (React/React Native),
// PyPI (Python), Go — run once per ecosystem. The export URL, the affected-package
// filter, and the applies_to label must all track the configured ecosystem (not
// hard-coded "Go"), or a non-Go advisory is dropped or mislabelled.
func TestOSVLiveSource_EcosystemParameterized(t *testing.T) {
	npm := map[string]any{
		"id":      "GHSA-npm-test-0001",
		"summary": "a distinctly worded npm advisory summary for this fixture only",
		"details": "a distinctly worded npm advisory details body for this fixture only",
		"affected": []map[string]any{{
			"package": map[string]string{"ecosystem": "npm", "name": "left-pad"},
			"ranges": []map[string]any{{
				"type":   "SEMVER",
				"events": []map[string]string{{"introduced": "0"}, {"fixed": "1.3.0"}},
			}},
		}},
	}
	src := ingest.NewOSVLiveSource(
		ingest.WithEcosystem("npm"),
		ingest.WithOSVLiveFetch(func(_ context.Context) (io.ReadCloser, error) {
			return buildOSVLiveZip(t, map[string]any{"GHSA-npm-test-0001.json": npm}), nil
		}),
	)
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("want 1 npm draft, got %d", len(drafts))
	}
	if len(drafts[0].AppliesTo) != 1 {
		t.Fatalf("applies_to len = %d, want 1", len(drafts[0].AppliesTo))
	}
	if a := drafts[0].AppliesTo[0]; a.Ecosystem != "npm" || a.Package != "left-pad" {
		t.Errorf("applies_to = %+v, want npm/left-pad", a)
	}
}
