// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/dotts-h/twiceshy/internal/record"
	"gopkg.in/yaml.v3"
)

// pyDeprecationsYAML is the curated, license-clean Python version-breaking set
// (the GitChameleon problem class). Distilled facts only (ADR-0003 §4).
//
//go:embed data/py-deprecations.yaml
var pyDeprecationsYAML []byte

// pyDeprecation is one curated fact: a PyPI library API that breaks across a
// version, the runtime diagnostic that flags it (the fingerprintable
// signature), and the replacement.
type pyDeprecation struct {
	Title      string `yaml:"title"`
	Summary    string `yaml:"summary"`
	Diagnostic string `yaml:"diagnostic"`
	Package    string `yaml:"package"`
	Introduced string `yaml:"introduced"`
	RootCause  string `yaml:"root_cause"`
	Fix        string `yaml:"fix"`
	Body       string `yaml:"body"`
	SourceURL  string `yaml:"source_url"`
}

type pySource struct{}

// NewPySource returns the Python version-breaking importer source.
func NewPySource() Source { return pySource{} }

func (pySource) Name() string { return "py" }

func (pySource) Drafts(_ context.Context) ([]Draft, error) {
	var deps []pyDeprecation
	if err := yaml.Unmarshal(pyDeprecationsYAML, &deps); err != nil {
		return nil, fmt.Errorf("py source: parse embedded data: %w", err)
	}
	drafts := make([]Draft, 0, len(deps))
	for _, d := range deps {
		var versions *record.VersionRange
		if d.Introduced != "" {
			introduced := d.Introduced
			versions = &record.VersionRange{Introduced: &introduced}
		}
		drafts = append(drafts, Draft{
			Kind:  "fix",
			Title: d.Title,
			Symptom: &record.Symptom{
				Summary:         d.Summary,
				ErrorSignatures: []string{d.Diagnostic},
			},
			AppliesTo: []record.AppliesTo{{
				Ecosystem: "PyPI",
				Package:   d.Package,
				Versions:  versions,
			}},
			Resolution:    &record.Resolution{RootCause: d.RootCause, Fix: d.Fix},
			Body:          d.Body,
			SourceLicense: record.SourceLicenseFactsOnly,
			SourceURL:     d.SourceURL,
		})
	}
	return drafts, nil
}
