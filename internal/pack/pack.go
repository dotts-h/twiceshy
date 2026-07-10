// SPDX-License-Identifier: AGPL-3.0-only

// Package pack builds distributable experience packs from validated records and
// mechanically keeps commercial packs license-clean (ADR-0002 §4, ADR-0003 §4):
// copyleft / share-alike / contract-encumbered sources are excluded from
// commercial packs, and copied licensed sources are included only with a
// source/license notice entry. This turns ADR-0002's licensing intent into a
// build-time check, not a manual audit.
//
// This is the pure core (classification + manifest); the file I/O lives in the
// twiceshy pack command (thin edge).
package pack

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dotts-h/twiceshy/internal/record"
)

// reCCBYAttribution matches the attribution-only CC-BY ids — "cc-by" or
// "cc-by-<version>" — but NOT the -SA / -NC / -ND variants, whose modifier
// letters follow "cc-by-" before any version. Only attribution-only CC-BY is
// commercial-safe (with attribution); every other CC variant is excluded.
var reCCBYAttribution = regexp.MustCompile(`^cc-by(-[0-9][0-9.]*)?$`)

// Eligibility is the commercial-pack verdict for one source_license.
type Eligibility struct {
	Commercial       bool   // may ship in a commercial pack
	NeedsAttribution bool   // if included, the pack must carry attribution
	Code             string // stable machine-readable reason bucket
	Reason           string // why — especially why a record is excluded
}

// Stable eligibility reason codes. Audit/report consumers key on these rather
// than parsing human prose.
const (
	ReasonMissingEvidence       = "missing_evidence"
	ReasonFactsOnly             = "facts_only"
	ReasonProjectAuthored       = "project_authored"
	ReasonAuthoredInternal      = "authored_internal"
	ReasonLicensedNotice        = "licensed_notice"
	ReasonCCBYNotice            = "cc_by_notice"
	ReasonRestrictedCC          = "restricted_cc"
	ReasonCopyleft              = "copyleft"
	ReasonUnrecognizedLicense   = "unrecognized_license"
	ReasonMissingSourceURL      = "missing_source_url"
	ReasonMissingNoticeEvidence = "missing_notice_evidence"
)

// permissiveLicenses impose no copyleft. A record carrying one of these IDs is
// treated as copied licensed material, so the pack still emits a source/license
// notice entry; "permissive" does not mean "obligation-free". Keys are
// lower-cased SPDX ids.
var permissiveLicenses = map[string]bool{
	"mit":          true,
	"apache-2.0":   true,
	"bsd-2-clause": true,
	"bsd-3-clause": true,
	"isc":          true,
	"0bsd":         true,
	"unlicense":    true,
	"cc0-1.0":      true,
}

// copyleftPrefixes are lower-cased SPDX id prefixes whose share-alike/copyleft
// obligations would infect a commercial pack.
var copyleftPrefixes = []string{"gpl-", "agpl-", "lgpl-", "mpl-", "epl-"}

// Classify decides whether a record carrying sourceLicense may ship in a
// commercial pack, and whether including it requires attribution. It is
// FAIL-CLOSED: an unrecognized license is excluded from commercial packs, so a
// new source can never silently leak an encumbered record into a paid pack.
func Classify(sourceLicense string) Eligibility {
	s := strings.TrimSpace(sourceLicense)
	switch s {
	case "":
		return Eligibility{Code: ReasonMissingEvidence, Reason: "missing explicit rights evidence — excluded fail-closed"}
	case record.SourceLicenseFactsOnly:
		return Eligibility{Commercial: true, Code: ReasonFactsOnly, Reason: "distilled facts only — no license obligation"}
	case record.SourceLicenseProjectAuthored:
		return Eligibility{Commercial: true, Code: ReasonProjectAuthored, Reason: "explicitly project-authored"}
	case record.SourceLicenseAuthoredInternal:
		// ADR-0011 §5: the fact was re-derived from a public-awareness topic for the
		// INTERNAL corpus only; the commercial pack stays gated on a real legal
		// review. Fail-closed — these never ship in a commercial pack until then.
		return Eligibility{Code: ReasonAuthoredInternal, Reason: "§5-authored, internal-only — pending commercial legal review (ADR-0011 §5)"}
	}

	low := strings.ToLower(s)
	if permissiveLicenses[low] {
		return Eligibility{Commercial: true, NeedsAttribution: true, Code: ReasonLicensedNotice, Reason: "permissive license (" + s + ") — source/license notice required"}
	}
	// Attribution-only CC-BY (no -SA/-NC/-ND modifier) is the one commercial-safe
	// CC variant; matched precisely so a modifier can never slip through.
	if reCCBYAttribution.MatchString(low) {
		return Eligibility{Commercial: true, NeedsAttribution: true, Code: ReasonCCBYNotice, Reason: "CC-BY (" + s + ") — attribution required"}
	}
	// Every other Creative Commons variant bars a commercial pack: -SA
	// (share-alike), -NC (noncommercial), -ND (no-derivatives), or an unknown CC
	// modifier. Fail-closed.
	if strings.HasPrefix(low, "cc-") {
		return Eligibility{Code: ReasonRestrictedCC, Reason: "restricted Creative Commons variant (" + s + ") — not commercial-safe"}
	}
	for _, p := range copyleftPrefixes {
		if strings.HasPrefix(low, p) {
			return Eligibility{Code: ReasonCopyleft, Reason: "copyleft (" + s + ")"}
		}
	}
	return Eligibility{Code: ReasonUnrecognizedLicense, Reason: "unrecognized license (" + s + ") — excluded fail-closed"}
}

// ClassifyRecord applies the complete commercial-pack rule to one record. A
// source license that carries a notice/attribution requirement is not enough by
// itself: the immutable source location needed to render that notice must also
// be present.
func ClassifyRecord(rec *record.Record) Eligibility {
	if rec == nil {
		return Eligibility{Code: ReasonMissingEvidence, Reason: "nil record — excluded fail-closed"}
	}
	e := Classify(rec.Provenance.SourceLicense)
	if e.Commercial && e.NeedsAttribution && strings.TrimSpace(rec.Provenance.SourceURL) == "" {
		e.Commercial = false
		e.Code = ReasonMissingSourceURL
		e.Reason = "source URL required for license/attribution notice"
		return e
	}
	if e.Commercial && e.NeedsAttribution && !completeSourceAttribution(rec) {
		e.Commercial = false
		e.Code = ReasonMissingNoticeEvidence
		e.Reason = "complete source attribution, notice, and license text evidence required"
	}
	return e
}

func completeSourceAttribution(rec *record.Record) bool {
	a := rec.Provenance.SourceAttribution
	if a == nil || strings.TrimSpace(a.LicenseText) == "" {
		return false
	}
	license := strings.ToLower(strings.TrimSpace(rec.Provenance.SourceLicense))
	if reCCBYAttribution.MatchString(license) {
		return allPresent(a.Creator, a.Title, a.LicenseURL, a.Changes)
	}
	switch license {
	case "mit":
		return allPresent(a.CopyrightNotice)
	case "apache-2.0":
		return allPresent(a.CopyrightNotice, a.Notice)
	default:
		// Other permissive licenses stay supported only with the conservative
		// superset: exact copyright, notice, and license material.
		return allPresent(a.CopyrightNotice, a.Notice)
	}
}

func allPresent(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}

// AttributionEntry records a source/license notice a pack must carry. For some
// licenses this is attribution; for permissive code licenses it ensures the
// source and applicable license/notice obligations are not silently dropped.
type AttributionEntry struct {
	ID              string `json:"id"`
	SourceLicense   string `json:"source_license"`
	SourceURL       string `json:"source_url"`
	Creator         string `json:"creator,omitempty"`
	Title           string `json:"title,omitempty"`
	LicenseURL      string `json:"license_url,omitempty"`
	Changes         string `json:"changes,omitempty"`
	CopyrightNotice string `json:"copyright_notice,omitempty"`
	Notice          string `json:"notice,omitempty"`
	LicenseText     string `json:"license_text"`
}

// Excluded records why a record was dropped from a pack.
type Excluded struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Manifest is the plan for a pack: which record ids are in, which are out (with
// reasons), and the attribution the pack must carry.
type Manifest struct {
	Commercial        bool               `json:"commercial"`
	PackLicenseSHA256 string             `json:"pack_license_sha256,omitempty"`
	Included          []string           `json:"included"`
	Excluded          []Excluded         `json:"excluded"`
	Attribution       []AttributionEntry `json:"attribution"`
}

// BuildManifest selects records for a pack. Packs ship `validated` records only
// (CONTEXT.md); includeQuarantined relaxes that for inspecting a not-yet-
// validated corpus. For a commercial pack, records whose source_license is not
// commercial-eligible (Classify) are additionally excluded, and every included
// record needing attribution is recorded. Pure and deterministic (sorted).
func BuildManifest(recs []*record.Record, commercial, includeQuarantined bool) Manifest {
	m := Manifest{Commercial: commercial}
	for _, r := range recs {
		if r.Status != "validated" && !includeQuarantined {
			m.Excluded = append(m.Excluded, Excluded{ID: r.ID, Reason: "not validated (status " + r.Status + ")"})
			continue
		}
		e := ClassifyRecord(r)
		if commercial && !e.Commercial {
			m.Excluded = append(m.Excluded, Excluded{ID: r.ID, Reason: e.Reason})
			continue
		}
		m.Included = append(m.Included, r.ID)
		if e.NeedsAttribution {
			a := r.Provenance.SourceAttribution
			entry := AttributionEntry{ID: r.ID, SourceLicense: r.Provenance.SourceLicense, SourceURL: r.Provenance.SourceURL}
			if a != nil {
				entry.Creator, entry.Title, entry.LicenseURL, entry.Changes = a.Creator, a.Title, a.LicenseURL, a.Changes
				entry.CopyrightNotice, entry.Notice, entry.LicenseText = a.CopyrightNotice, a.Notice, a.LicenseText
			}
			m.Attribution = append(m.Attribution, entry)
		}
	}
	sort.Strings(m.Included)
	sort.Slice(m.Excluded, func(i, j int) bool { return m.Excluded[i].ID < m.Excluded[j].ID })
	sort.Slice(m.Attribution, func(i, j int) bool { return m.Attribution[i].ID < m.Attribution[j].ID })
	return m
}

// NoticeDocument renders the canonical source/license notice artifact for a
// manifest. Keeping this beside BuildManifest lets pre-ship validation compare
// the exact artifact without duplicating rendering policy at the CLI edge.
func NoticeDocument(m Manifest) []byte {
	var b strings.Builder
	b.WriteString("# Source and License Notices\n\n")
	if len(m.Attribution) == 0 {
		b.WriteString("No records in this pack require a source/license notice entry.\n")
		return []byte(b.String())
	}
	b.WriteString("This pack includes records from the following licensed sources. Preserve the applicable attribution, copyright, license, and NOTICE terms identified by each source and license:\n\n")
	for _, a := range m.Attribution {
		_, _ = fmt.Fprintf(&b, "## %s\n\n- Source: %s\n- License: %s\n", a.ID, a.SourceURL, a.SourceLicense)
		if a.Creator != "" {
			_, _ = fmt.Fprintf(&b, "- Creator: %s\n- Work title: %s\n- License link: %s\n- Changes: %s\n", a.Creator, a.Title, a.LicenseURL, a.Changes)
		}
		if a.CopyrightNotice != "" {
			_, _ = fmt.Fprintf(&b, "- Copyright notice: %s\n", a.CopyrightNotice)
		}
		if a.Notice != "" {
			_, _ = fmt.Fprintf(&b, "\n### Upstream NOTICE\n\n%s\n", a.Notice)
		}
		_, _ = fmt.Fprintf(&b, "\n### License text\n\n%s\n\n", a.LicenseText)
	}
	return []byte(b.String())
}

// ValidateCommercialArtifacts verifies that a built commercial MANIFEST.json
// and notice document exactly match the current corpus and pack policy. It is
// deterministic and returns every drift finding in a stable order.
func ValidateCommercialArtifacts(recs []*record.Record, got Manifest, notices, packLicense []byte) []string {
	want := BuildManifest(recs, true, false)
	var errs []string
	if len(bytes.TrimSpace(packLicense)) == 0 {
		errs = append(errs, "pack-level LICENSE terms are missing or empty")
	} else {
		want.PackLicenseSHA256 = LicenseDigest(packLicense)
	}
	gotJSON, gotErr := json.Marshal(got)
	wantJSON, wantErr := json.Marshal(want)
	if gotErr != nil || wantErr != nil || !bytes.Equal(gotJSON, wantJSON) {
		errs = append(errs, "MANIFEST.json does not match the current commercial pack selection and notice ledger")
	}
	if !bytes.Equal(notices, NoticeDocument(want)) {
		errs = append(errs, "source/license notice document does not match the current commercial manifest")
	}
	return errs
}

// LicenseDigest binds MANIFEST.json to the exact pack-level LICENSE terms.
func LicenseDigest(terms []byte) string {
	sum := sha256.Sum256(terms)
	return fmt.Sprintf("sha256:%x", sum)
}
