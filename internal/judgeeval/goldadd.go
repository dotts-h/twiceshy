// SPDX-License-Identifier: AGPL-3.0-only

package judgeeval

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/record"
	"gopkg.in/yaml.v3"
)

// GoldStanzaInput is the operator-supplied fields for one new gold.yaml case.
type GoldStanzaInput struct {
	ID          string
	Mode        string
	Rationale   string
	Checks      []string // want_failing_checks (reject only; empty for approve)
	Record      *record.Record
	Repros      []judge.ReproArtifact
	Attestation struct {
		Holds, Inconclusive bool
		ReproducedUnder     []string
	}
}

// GoldCaseStanza renders ONE `cases:` entry as YAML (2-space indent matching
// gold.yaml), deriving want_decision from Mode (approve→approve, else reject).
// It validates the input against the same rules LoadGold enforces.
func GoldCaseStanza(in GoldStanzaInput) (string, error) {
	where := in.ID
	if where == "" {
		where = "(new case)"
	}
	if err := validateStanzaInput(where, in); err != nil {
		return "", err
	}

	wantDecision := "reject"
	if in.Mode == "approve" {
		wantDecision = "approve"
	}

	type reproYAML struct {
		Path    string `yaml:"path"`
		Kind    string `yaml:"kind"`
		Label   string `yaml:"label,omitempty"`
		Content string `yaml:"content"`
	}
	repros := make([]reproYAML, len(in.Repros))
	for i, rp := range in.Repros {
		repros[i] = reproYAML{Path: rp.Path, Kind: rp.Kind, Label: rp.Label, Content: rp.Content}
	}

	caseYAML := struct {
		ID                string         `yaml:"id"`
		Mode              string         `yaml:"mode"`
		WantDecision      string         `yaml:"want_decision"`
		WantFailingChecks []string       `yaml:"want_failing_checks,omitempty"`
		Rationale         string         `yaml:"rationale"`
		Record            *record.Record `yaml:"record"`
		Attestation       struct {
			Holds           bool     `yaml:"holds"`
			Inconclusive    bool     `yaml:"inconclusive"`
			ReproducedUnder []string `yaml:"reproduced_under,omitempty"`
		} `yaml:"attestation"`
		Repros []reproYAML `yaml:"repros"`
	}{
		ID:           in.ID,
		Mode:         in.Mode,
		WantDecision: wantDecision,
		Rationale:    in.Rationale,
		Record:       in.Record,
		Repros:       repros,
	}
	caseYAML.Attestation.Holds = in.Attestation.Holds
	caseYAML.Attestation.Inconclusive = in.Attestation.Inconclusive
	caseYAML.Attestation.ReproducedUnder = in.Attestation.ReproducedUnder
	if wantDecision == "reject" {
		caseYAML.WantFailingChecks = append([]string(nil), in.Checks...)
	}

	wrapper := struct {
		Cases []any `yaml:"cases"`
	}{Cases: []any{caseYAML}}

	b, err := yaml.Marshal(&wrapper)
	if err != nil {
		return "", fmt.Errorf("%s: marshaling stanza: %w", where, err)
	}
	const prefix = "cases:\n"
	s := string(b)
	if !strings.HasPrefix(s, prefix) {
		return "", fmt.Errorf("%s: unexpected yaml marshal prefix", where)
	}
	return s[len(prefix):], nil
}

func validateStanzaInput(where string, in GoldStanzaInput) error {
	var errs []error
	if in.ID == "" {
		errs = append(errs, fmt.Errorf("%s: id is required", where))
	}
	if !validMode(in.Mode) {
		errs = append(errs, fmt.Errorf("%s: unknown mode %q", where, in.Mode))
	}
	if in.Record == nil || in.Record.Title == "" {
		errs = append(errs, fmt.Errorf("%s: record with a title is required", where))
	}
	if len(in.Repros) == 0 {
		errs = append(errs, fmt.Errorf("%s: at least one repro is required (the judge reads it)", where))
	}
	if in.Rationale == "" {
		errs = append(errs, fmt.Errorf("%s: a ground-truth rationale is required", where))
	}

	wantDecision := judge.Reject
	if in.Mode == "approve" {
		wantDecision = judge.Approve
	}
	checks, checkErr := parseChecks(where, in.Checks)
	errs = append(errs, checkErr...)

	switch wantDecision {
	case judge.Approve:
		if len(checks) > 0 {
			errs = append(errs, fmt.Errorf("%s: an approve case must list no failing checks", where))
		}
		if in.Mode != "approve" {
			errs = append(errs, fmt.Errorf("%s: want_decision approve requires mode approve, got %q", where, in.Mode))
		}
	case judge.Reject:
		if len(checks) == 0 {
			errs = append(errs, fmt.Errorf("%s: a reject case must name at least one failing check", where))
		}
		if in.Mode == "approve" {
			errs = append(errs, fmt.Errorf("%s: mode approve cannot be a reject case", where))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
