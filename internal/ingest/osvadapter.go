// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/dotts-h/twiceshy/internal/record"
	"gopkg.in/yaml.v3"
)

// osvAdvisoriesYAML is the curated GitHub Advisory Database / OSV projection.
// GHSA is CC-BY-4.0, so these records carry that license + attribution; only
// functional identifiers (GHSA/CVE ids, package names, version ranges) are
// verbatim (ADR-0003 §4).
//
//go:embed data/osv-advisories.yaml
var osvAdvisoriesYAML []byte

// osvAffected is the OSV affected[].ranges projection: one affected package and
// the introduced/fixed version events, taken verbatim.
type osvAffected struct {
	Ecosystem  string `yaml:"ecosystem"`
	Package    string `yaml:"package"`
	Introduced string `yaml:"introduced"`
	Fixed      string `yaml:"fixed"`
}

// osvAdvisory is one curated advisory. Prose is distilled in twiceshy's own
// words; the GHSA/CVE ids, package names and version ranges are verbatim.
type osvAdvisory struct {
	GHSA      string        `yaml:"ghsa"`
	CVE       string        `yaml:"cve"`
	URL       string        `yaml:"url"`
	Title     string        `yaml:"title"`
	Summary   string        `yaml:"summary"`
	RootCause string        `yaml:"root_cause"`
	Fix       string        `yaml:"fix"`
	Body      string        `yaml:"body"`
	Affected  []osvAffected `yaml:"affected"`
}

type osvSource struct{}

// NewOSVSource returns the OSV / GitHub Advisory Database importer source.
func NewOSVSource() Source { return osvSource{} }

func (osvSource) Name() string { return "osv" }

// Drafts emits one trap draft per advisory: the symptom carries the GHSA/CVE
// ids as fingerprintable error signatures, applies_to maps the OSV affected
// ranges near 1:1, and the record is licensed CC-BY-4.0 with a source_url.
func (osvSource) Drafts(_ context.Context) ([]Draft, error) {
	var advs []osvAdvisory
	if err := yaml.Unmarshal(osvAdvisoriesYAML, &advs); err != nil {
		return nil, fmt.Errorf("osv source: parse embedded data: %w", err)
	}
	drafts := make([]Draft, 0, len(advs))
	for _, a := range advs {
		var sigs []string
		if a.GHSA != "" {
			sigs = append(sigs, a.GHSA)
		}
		if a.CVE != "" {
			sigs = append(sigs, a.CVE)
		}
		applies := make([]record.AppliesTo, 0, len(a.Affected))
		for _, af := range a.Affected {
			applies = append(applies, record.AppliesTo{
				Ecosystem: af.Ecosystem,
				Package:   af.Package,
				Versions:  versionRange(af.Introduced, af.Fixed),
			})
		}
		drafts = append(drafts, buildOSVDraft(osvDraftInput{
			Signatures: sigs,
			AppliesTo:  applies,
			Title:      a.Title,
			Summary:    a.Summary,
			RootCause:  a.RootCause,
			Fix:        a.Fix,
			Body:       a.Body,
			SourceURL:  a.URL,
		}))
	}
	return drafts, nil
}
