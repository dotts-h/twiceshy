// SPDX-License-Identifier: AGPL-3.0-only

// prospect_gold.go implements the #0114 gold-emission surface: model-hard cases
// the prospector found become replayable #0005 gold tasks, so the ON-arm delta
// (does the card actually help) can be re-measured as models change over time.
package agenteval

import (
	_ "embed"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

//go:embed prospect_gold.yaml
var prospectGoldYAML []byte

// prospectGoldFile mirrors prospect_gold.yaml / any file MergeProspectGold writes.
type prospectGoldFile struct {
	Cases []prospectGoldCaseYAML `yaml:"cases"`
}

type prospectGoldCaseYAML struct {
	TrapID   string   `yaml:"trap_id"`
	Prompt   string   `yaml:"prompt"`
	Card     string   `yaml:"card,omitempty"`
	VerifyID string   `yaml:"verify_id"`
	Deps     []string `yaml:"deps,omitempty"`
}

func (c prospectGoldCaseYAML) toTaskCase() TaskCase {
	return TaskCase{TrapID: c.TrapID, Prompt: c.Prompt, Card: c.Card, VerifyID: c.VerifyID, Deps: c.Deps}
}

func prospectGoldCaseFrom(pc ProspectCase) prospectGoldCaseYAML {
	return prospectGoldCaseYAML{TrapID: pc.TrapID, Prompt: pc.Prompt, Card: pc.Card, VerifyID: pc.VerifyID, Deps: pc.Deps}
}

// LoadProspectGold parses the embedded prospect_gold.yaml into TaskCases, ready to
// extend GoldTasks() for the #0005 eval's task iteration. An empty file (no cases
// drafted yet) is tolerated: zero cases, no error.
func LoadProspectGold() ([]TaskCase, error) {
	gf, err := parseProspectGoldYAML(prospectGoldYAML)
	if err != nil {
		return nil, fmt.Errorf("agenteval: parsing prospect_gold.yaml: %w", err)
	}
	cases := make([]TaskCase, 0, len(gf.Cases))
	for _, c := range gf.Cases {
		cases = append(cases, c.toTaskCase())
	}
	return cases, nil
}

// loadProspectGoldFrom reads and parses an on-disk gold file at path, the
// MergeProspectGold-side counterpart to LoadProspectGold's embedded read. A
// missing file is tolerated (empty gold set, e.g. a fresh -gold-out target).
func loadProspectGoldFrom(path string) ([]TaskCase, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("agenteval: reading %s: %w", path, err)
	}
	gf, err := parseProspectGoldYAML(b)
	if err != nil {
		return nil, fmt.Errorf("agenteval: parsing %s: %w", path, err)
	}
	cases := make([]TaskCase, 0, len(gf.Cases))
	for _, c := range gf.Cases {
		cases = append(cases, c.toTaskCase())
	}
	return cases, nil
}

func parseProspectGoldYAML(b []byte) (prospectGoldFile, error) {
	var gf prospectGoldFile
	if err := yaml.Unmarshal(b, &gf); err != nil {
		return prospectGoldFile{}, err
	}
	return gf, nil
}

// allProspectAndGoldTasks merges the hand-authored GoldTasks() with any
// prospector-emitted gold cases (#0114), so a live task iteration exercises a
// model-hard case the prospector found too. LoadProspectGold already tolerates
// an absent/empty prospect_gold.yaml (zero cases, no error); this wrapper
// additionally tolerates any load error the same way (skip-if-absent) so a
// prospect-gold hiccup never blocks the hand-authored gold tasks from running.
func allProspectAndGoldTasks() []TaskCase {
	tasks := GoldTasks()
	if pg, err := LoadProspectGold(); err == nil {
		tasks = append(tasks, pg...)
	}
	return tasks
}

// MergeProspectGold appends cases to the gold file at path, deduped by TrapID —
// an entry already present at path wins over an incoming one with the same
// TrapID (a re-run never overwrites a previously-committed gold case). A missing
// path is tolerated (starts from an empty set); the result is written back to
// path as YAML.
func MergeProspectGold(path string, cases []ProspectCase) error {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("agenteval: reading %s: %w", path, err)
	}
	var gf prospectGoldFile
	if err == nil {
		gf, err = parseProspectGoldYAML(b)
		if err != nil {
			return fmt.Errorf("agenteval: parsing %s: %w", path, err)
		}
	}

	seen := make(map[string]bool, len(gf.Cases))
	for _, c := range gf.Cases {
		seen[c.TrapID] = true
	}
	for _, pc := range cases {
		if seen[pc.TrapID] {
			continue // existing entries win
		}
		gf.Cases = append(gf.Cases, prospectGoldCaseFrom(pc))
		seen[pc.TrapID] = true
	}

	out, err := yaml.Marshal(gf)
	if err != nil {
		return fmt.Errorf("agenteval: marshaling %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("agenteval: writing %s: %w", path, err)
	}
	return nil
}
