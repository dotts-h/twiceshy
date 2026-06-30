// SPDX-License-Identifier: AGPL-3.0-only

package judgeeval

import (
	"fmt"
	"io"
	"strings"

	"github.com/dotts-h/twiceshy/internal/judge"
)

// Config is one prompt×reasoning combination the A/B sweeps.
type Config struct {
	Name   string
	System string
	Think  bool
}

// Configs is the prompt×reasoning table judge-eval sweeps.
var Configs = []Config{
	{"prose-nothink", judge.ProseSystemV1, false},
	{"prose-think", judge.ProseSystemV1, true},
	{"rubric-nothink", judge.RubricSystemV1, false},
	{"rubric-think", judge.RubricSystemV1, true},
}

// NamedResult pairs a config name with its scored result (for the report table
// and the JSON payload).
type NamedResult struct {
	Name   string `json:"config"`
	Result Result `json:"result"`
}

// ConfigNames returns the comma-separated config names for flag help.
func ConfigNames() string {
	ns := make([]string, len(Configs))
	for i, c := range Configs {
		ns[i] = c.Name
	}
	return strings.Join(ns, ",")
}

// SelectConfigs resolves a comma list of config names, or all when spec is empty
// or "all".
func SelectConfigs(spec string) ([]Config, error) {
	if spec == "" || spec == "all" {
		return Configs, nil
	}
	want := strings.Split(spec, ",")
	var out []Config
	for _, w := range want {
		w = strings.TrimSpace(w)
		found := false
		for _, c := range Configs {
			if c.Name == w {
				out = append(out, c)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown config %q (want one of %s, or all)", w, ConfigNames())
		}
	}
	return out, nil
}

// Better ranks results: fewest false-approves (fail-unsafe) first, then fewest
// false-rejects, then fewest errors, then highest accuracy.
func Better(a, b Result) bool {
	if a.FalseApproves != b.FalseApproves {
		return a.FalseApproves < b.FalseApproves
	}
	if a.FalseRejects != b.FalseRejects {
		return a.FalseRejects < b.FalseRejects
	}
	if a.Errors != b.Errors {
		return a.Errors < b.Errors
	}
	return a.Accuracy > b.Accuracy
}

// ResultName returns the config name at index i, or "(none)" when out of range.
func ResultName(results []NamedResult, i int) string {
	if i < 0 || i >= len(results) {
		return "(none)"
	}
	return results[i].Name
}

// PrintMisses writes a one-line summary of gold cases matching pred.
func PrintMisses(out io.Writer, title string, outcomes []Outcome, pred func(Outcome) bool) {
	var ids []string
	for _, o := range outcomes {
		if pred(o) {
			ids = append(ids, fmt.Sprintf("%s(%s)", o.CaseID, o.Mode))
		}
	}
	if len(ids) > 0 {
		_, _ = fmt.Fprintf(out, "  %s: %s\n", title, strings.Join(ids, " "))
	}
}
