// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"github.com/dotts-h/twiceshy/internal/record"
)

// ghsaLicense is the SPDX id GHSA content is published under; the pack builder
// (later) treats CC-BY records as include-with-attribution.
const ghsaLicense = "CC-BY-4.0"

// osvDraftInput is the distilled, license-clean input to an OSV trap draft.
// Callers supply the prose: the embedded adapter passes curated prose; the
// live importer passes GENERATED prose (never OSV's copyrighted text).
type osvDraftInput struct {
	Signatures []string // error_signatures: advisory id + aliases (verbatim functional ids only)
	AppliesTo  []record.AppliesTo
	Title      string
	Summary    string
	RootCause  string
	Fix        string
	Body       string
	SourceURL  string
}

// buildOSVDraft assembles the invariant OSV trap-draft shape (Kind="trap",
// SourceLicense=CC-BY-4.0); the caller supplies facts + prose.
func buildOSVDraft(in osvDraftInput) Draft {
	return Draft{
		Kind:          "trap",
		Title:         in.Title,
		Symptom:       &record.Symptom{Summary: in.Summary, ErrorSignatures: in.Signatures},
		AppliesTo:     in.AppliesTo,
		Resolution:    &record.Resolution{RootCause: in.RootCause, Fix: in.Fix},
		Body:          in.Body,
		SourceLicense: ghsaLicense,
		SourceURL:     in.SourceURL,
	}
}

// versionRange builds an *record.VersionRange from optional introduced/fixed
// strings, returning nil when both are empty.
func versionRange(introduced, fixed string) *record.VersionRange {
	if introduced == "" && fixed == "" {
		return nil
	}
	vr := &record.VersionRange{}
	if introduced != "" {
		vr.Introduced = &introduced
	}
	if fixed != "" {
		vr.Fixed = &fixed
	}
	return vr
}
