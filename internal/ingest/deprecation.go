// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"context"
	"fmt"

	"github.com/dotts-h/twiceshy/internal/record"
	"gopkg.in/yaml.v3"
)

// deprecation is one curated, license-clean fact: an API that breaks across a
// version, the runtime/lint diagnostic that flags it (the fingerprintable
// signature), and the replacement. Shared by the Go and Python sources, which
// differ only in their embedded data and ecosystem label.
type deprecation struct {
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

// deprecationSource maps an embedded set of curated deprecation facts to
// quarantined-record Drafts (distilled facts only, ADR-0003 §4).
type deprecationSource struct {
	name      string
	ecosystem string
	data      []byte
}

func (s deprecationSource) Name() string { return s.name }

func (s deprecationSource) Drafts(_ context.Context) ([]Draft, error) {
	var deps []deprecation
	if err := yaml.Unmarshal(s.data, &deps); err != nil {
		return nil, fmt.Errorf("%s source: parse embedded data: %w", s.name, err)
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
				Ecosystem: s.ecosystem,
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
