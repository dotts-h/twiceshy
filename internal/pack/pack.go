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
	Reason           string // why — especially why a record is excluded
}

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
		return Eligibility{Reason: "missing explicit rights evidence — excluded fail-closed"}
	case record.SourceLicenseFactsOnly:
		return Eligibility{Commercial: true, Reason: "distilled facts only — no license obligation"}
	case record.SourceLicenseProjectAuthored:
		return Eligibility{Commercial: true, Reason: "explicitly project-authored"}
	case record.SourceLicenseAuthoredInternal:
		// ADR-0011 §5: the fact was re-derived from a public-awareness topic for the
		// INTERNAL corpus only; the commercial pack stays gated on a real legal
		// review. Fail-closed — these never ship in a commercial pack until then.
		return Eligibility{Reason: "§5-authored, internal-only — pending commercial legal review (ADR-0011 §5)"}
	}

	low := strings.ToLower(s)
	if permissiveLicenses[low] {
		return Eligibility{Commercial: true, NeedsAttribution: true, Reason: "permissive license (" + s + ") — source/license notice required"}
	}
	// Attribution-only CC-BY (no -SA/-NC/-ND modifier) is the one commercial-safe
	// CC variant; matched precisely so a modifier can never slip through.
	if reCCBYAttribution.MatchString(low) {
		return Eligibility{Commercial: true, NeedsAttribution: true, Reason: "CC-BY (" + s + ") — attribution required"}
	}
	// Every other Creative Commons variant bars a commercial pack: -SA
	// (share-alike), -NC (noncommercial), -ND (no-derivatives), or an unknown CC
	// modifier. Fail-closed.
	if strings.HasPrefix(low, "cc-") {
		return Eligibility{Reason: "restricted Creative Commons variant (" + s + ") — not commercial-safe"}
	}
	for _, p := range copyleftPrefixes {
		if strings.HasPrefix(low, p) {
			return Eligibility{Reason: "copyleft (" + s + ")"}
		}
	}
	return Eligibility{Reason: "unrecognized license (" + s + ") — excluded fail-closed"}
}

// AttributionEntry records a source/license notice a pack must carry. For some
// licenses this is attribution; for permissive code licenses it ensures the
// source and applicable license/notice obligations are not silently dropped.
type AttributionEntry struct {
	ID            string `json:"id"`
	SourceLicense string `json:"source_license"`
	SourceURL     string `json:"source_url"`
}

// Excluded records why a record was dropped from a pack.
type Excluded struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Manifest is the plan for a pack: which record ids are in, which are out (with
// reasons), and the attribution the pack must carry.
type Manifest struct {
	Commercial  bool               `json:"commercial"`
	Included    []string           `json:"included"`
	Excluded    []Excluded         `json:"excluded"`
	Attribution []AttributionEntry `json:"attribution"`
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
		e := Classify(r.Provenance.SourceLicense)
		if commercial && !e.Commercial {
			m.Excluded = append(m.Excluded, Excluded{ID: r.ID, Reason: e.Reason})
			continue
		}
		if commercial && e.NeedsAttribution && strings.TrimSpace(r.Provenance.SourceURL) == "" {
			m.Excluded = append(m.Excluded, Excluded{ID: r.ID, Reason: "source URL required for license/attribution notice"})
			continue
		}
		m.Included = append(m.Included, r.ID)
		if e.NeedsAttribution {
			m.Attribution = append(m.Attribution, AttributionEntry{
				ID:            r.ID,
				SourceLicense: r.Provenance.SourceLicense,
				SourceURL:     r.Provenance.SourceURL,
			})
		}
	}
	sort.Strings(m.Included)
	sort.Slice(m.Excluded, func(i, j int) bool { return m.Excluded[i].ID < m.Excluded[j].ID })
	sort.Slice(m.Attribution, func(i, j int) bool { return m.Attribution[i].ID < m.Attribution[j].ID })
	return m
}
