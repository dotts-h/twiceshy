// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/dotts-h/twiceshy/internal/record"
	"gopkg.in/yaml.v3"
)

// goDeprecationsYAML is the curated, license-clean Go stdlib deprecation set.
// Distilled facts only (ADR-0003 §4).
//
//go:embed data/go-deprecations.yaml
var goDeprecationsYAML []byte

// goDeprecation is one curated fact: a deprecated stdlib API, the staticcheck
// SA1019 diagnostic that flags it (the fingerprintable signature), and the
// replacement.
type goDeprecation struct {
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

type goSource struct{}

// NewGoSource returns the Go stdlib deprecation importer source.
func NewGoSource() Source { return goSource{} }

func (goSource) Name() string { return "go" }

func (goSource) Drafts(_ context.Context) ([]Draft, error) {
	var deps []goDeprecation
	if err := yaml.Unmarshal(goDeprecationsYAML, &deps); err != nil {
		return nil, fmt.Errorf("go source: parse embedded data: %w", err)
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
				Ecosystem: "Go",
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
