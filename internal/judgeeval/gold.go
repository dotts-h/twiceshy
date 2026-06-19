// SPDX-License-Identifier: AGPL-3.0-only

// Package judgeeval is the judge-prompt eval (ADR-0013 #0028): it measures how
// well the diverse-model judge separates promotable records from ones it must
// reject, across every failure mode a green attestation cannot catch (meaning,
// scope, license, poison). It exists because the judge's strictness was first
// hand-tuned by guessing (see the shim README's "Tuning" note); this package
// replaces that with a labelled gold set + a measured A/B of prompt and reasoning
// settings, scoring the fail-UNSAFE direction (false-approve rate).
//
// The gold set (gold.yaml, embedded) is hand-labelled ground truth. The scoring
// engine (eval.go) is deterministic and unit-tested with a stub judge — no network
// in CI. The live A/B drives the real production judge seam (judge.ModelJudge)
// against the off-pool shim and is gated behind an endpoint, never run in CI.
package judgeeval

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
	"gopkg.in/yaml.v3"
)

//go:embed gold.yaml
var goldYAML []byte

// Case is one labelled gold case: a judging request plus the ground-truth verdict
// Claude assigned to it. Mode names the failure mode the case exercises; for an
// approve case it is "approve" and WantFailingChecks is empty.
type Case struct {
	ID                string
	Mode              string
	WantDecision      judge.Decision
	WantFailingChecks []judge.CheckName
	Rationale         string

	record      *record.Record
	attestation repro.Attestation
	repros      []judge.ReproArtifact
}

// Request renders the case into the judging request the judge actually sees —
// the same shape twiceshy's promote/adapt paths build.
func (c Case) Request() judge.Request {
	return judge.Request{Record: c.record, Attestation: c.attestation, Repros: c.repros}
}

// ShouldReject reports whether ground truth wants this case rejected — the cases
// that make a false-approve possible (the fail-unsafe direction we score).
func (c Case) ShouldReject() bool { return c.WantDecision == judge.Reject }

// goldFile mirrors gold.yaml. The record decodes straight into record.Record via
// its yaml tags (we read only the fields the judge renders, so we do not run the
// full record validator — these are deliberately partial fixtures).
type goldFile struct {
	Cases []struct {
		ID                string         `yaml:"id"`
		Mode              string         `yaml:"mode"`
		WantDecision      string         `yaml:"want_decision"`
		WantFailingChecks []string       `yaml:"want_failing_checks"`
		Rationale         string         `yaml:"rationale"`
		Record            *record.Record `yaml:"record"`
		Attestation       struct {
			Holds           bool     `yaml:"holds"`
			Inconclusive    bool     `yaml:"inconclusive"`
			ReproducedUnder []string `yaml:"reproduced_under"`
		} `yaml:"attestation"`
		Repros []struct {
			Path    string `yaml:"path"`
			Kind    string `yaml:"kind"`
			Label   string `yaml:"label"`
			Content string `yaml:"content"`
		} `yaml:"repros"`
	} `yaml:"cases"`
}

// LoadGold parses and validates the embedded gold set. It fails loudly on any
// internal inconsistency (an approve case carrying failing checks, a reject case
// with none, an unknown mode/check, a duplicate id) so a malformed gold set can
// never silently weaken the eval.
func LoadGold() ([]Case, error) {
	var gf goldFile
	if err := yaml.Unmarshal(goldYAML, &gf); err != nil {
		return nil, fmt.Errorf("judgeeval: parsing gold.yaml: %w", err)
	}
	if len(gf.Cases) == 0 {
		return nil, errors.New("judgeeval: gold.yaml has no cases")
	}
	seen := make(map[string]bool, len(gf.Cases))
	cases := make([]Case, 0, len(gf.Cases))
	var errs []error
	for i, gc := range gf.Cases {
		where := gc.ID
		if where == "" {
			where = fmt.Sprintf("cases[%d]", i)
		}
		if gc.ID == "" {
			errs = append(errs, fmt.Errorf("%s: id is required", where))
		}
		if seen[gc.ID] {
			errs = append(errs, fmt.Errorf("%s: duplicate id", where))
		}
		seen[gc.ID] = true
		if !validMode(gc.Mode) {
			errs = append(errs, fmt.Errorf("%s: unknown mode %q", where, gc.Mode))
		}
		if gc.Record == nil || gc.Record.Title == "" {
			errs = append(errs, fmt.Errorf("%s: record with a title is required", where))
		}
		if len(gc.Repros) == 0 {
			errs = append(errs, fmt.Errorf("%s: at least one repro is required (the judge reads it)", where))
		}
		if gc.Rationale == "" {
			errs = append(errs, fmt.Errorf("%s: a ground-truth rationale is required", where))
		}

		dec := judge.Decision(gc.WantDecision)
		if dec != judge.Approve && dec != judge.Reject {
			errs = append(errs, fmt.Errorf("%s: want_decision %q must be approve or reject", where, gc.WantDecision))
		}
		checks, checkErr := parseChecks(where, gc.WantFailingChecks)
		errs = append(errs, checkErr...)

		// Ground-truth consistency: approve ⇔ no failing checks; reject ⇔ ≥1.
		switch dec {
		case judge.Approve:
			if len(checks) > 0 {
				errs = append(errs, fmt.Errorf("%s: an approve case must list no failing checks", where))
			}
			if gc.Mode != "approve" {
				errs = append(errs, fmt.Errorf("%s: want_decision approve requires mode approve, got %q", where, gc.Mode))
			}
		case judge.Reject:
			if len(checks) == 0 {
				errs = append(errs, fmt.Errorf("%s: a reject case must name at least one failing check", where))
			}
			if gc.Mode == "approve" {
				errs = append(errs, fmt.Errorf("%s: mode approve cannot be a reject case", where))
			}
		}

		rec := gc.Record
		var att repro.Attestation
		var arts []judge.ReproArtifact
		if rec != nil {
			att = repro.Attestation{
				RecordID:        rec.ID,
				Holds:           gc.Attestation.Holds,
				Inconclusive:    gc.Attestation.Inconclusive,
				ReproducedUnder: gc.Attestation.ReproducedUnder,
			}
		}
		for _, rp := range gc.Repros {
			arts = append(arts, judge.ReproArtifact{Path: rp.Path, Kind: rp.Kind, Label: rp.Label, Content: rp.Content})
		}
		cases = append(cases, Case{
			ID:                gc.ID,
			Mode:              gc.Mode,
			WantDecision:      dec,
			WantFailingChecks: checks,
			Rationale:         gc.Rationale,
			record:            rec,
			attestation:       att,
			repros:            arts,
		})
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cases, nil
}

// Modes are the failure modes the gold set spans. "approve" is the clean control;
// the rest each name the check a reject case is expected to fail on.
var Modes = []string{"approve", "poison", "scope", "meaning", "license"}

func validMode(m string) bool {
	for _, v := range Modes {
		if v == m {
			return true
		}
	}
	return false
}

func parseChecks(where string, names []string) ([]judge.CheckName, []error) {
	var out []judge.CheckName
	var errs []error
	seen := make(map[judge.CheckName]bool, len(names))
	for _, n := range names {
		c := judge.CheckName(n)
		if !knownCheck(c) {
			errs = append(errs, fmt.Errorf("%s: unknown check %q", where, n))
			continue
		}
		if seen[c] {
			errs = append(errs, fmt.Errorf("%s: duplicate check %q", where, n))
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out, errs
}

func knownCheck(c judge.CheckName) bool {
	for _, k := range judge.Checks {
		if k == c {
			return true
		}
	}
	return false
}
