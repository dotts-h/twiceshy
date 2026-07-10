// SPDX-License-Identifier: AGPL-3.0-only

package measurement_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/measurement"
)

func TestLoadInputsAreStrictAndPrivacySafe(t *testing.T) {
	dir := t.TempDir()
	cohorts := filepath.Join(dir, "cohorts.csv")
	if err := os.WriteFile(cohorts, []byte("team,session_hash\nteam-a,0123456789abcdef0123456789abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := measurement.LoadCohorts(cohorts)
	if err != nil || got["0123456789abcdef0123456789abcdef"] != "team-a" {
		t.Fatalf("LoadCohorts = %v, %v", got, err)
	}

	outcomes := filepath.Join(dir, "outcomes.jsonl")
	line := `{"ts":"2026-07-08T04:00:00Z","session_hash":"0123456789abcdef0123456789abcdef","record_id":"exp-0001","used":true,"confirmed":true}` + "\n"
	if err := os.WriteFile(outcomes, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	outs, err := measurement.LoadOutcomes(outcomes)
	if err != nil || len(outs) != 1 || outs[0].Used == nil || !*outs[0].Used {
		t.Fatalf("LoadOutcomes = %+v, %v", outs, err)
	}

	if err := os.WriteFile(outcomes, []byte(`{"ts":"2026-07-08T04:00:00Z","session_hash":"0123456789abcdef0123456789abcdef","record_id":"exp-0001","evidence":"secret"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := measurement.LoadOutcomes(outcomes); err == nil {
		t.Fatal("unknown raw evidence field must be rejected")
	}
}

func TestLoadDecisionsFailsClosedAndSupportsExplicitArchives(t *testing.T) {
	dir := t.TempDir()
	good := func(ts string) string {
		return `{"ts":"` + ts + `","channel":"push","query_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","session":"0123456789abcdef0123456789abcdef","count":0,"trigger":"error"}` + "\n"
	}
	a, b := filepath.Join(dir, "archive.jsonl"), filepath.Join(dir, "active.jsonl")
	if err := os.WriteFile(a, []byte(good("2026-07-01T00:00:00Z")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(good("2026-07-08T00:00:00Z")), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := measurement.LoadDecisions([]string{a, b})
	if err != nil || len(got) != 2 {
		t.Fatalf("LoadDecisions = %d, %v", len(got), err)
	}
	cases := map[string]string{
		"missing": filepath.Join(dir, "missing.jsonl"), "empty": "", "corrupt": "not-json\n", "timestamp": good("not-a-time"),
		"raw-query": strings.Replace(good("2026-07-01T00:00:00Z"), `"count":0`, `"query_text":"secret","count":0`, 1),
		"unknown":   strings.Replace(good("2026-07-01T00:00:00Z"), `"count":0`, `"evidence":"secret","count":0`, 1),
	}
	for name, fixture := range cases {
		t.Run(name, func(t *testing.T) {
			path := fixture
			if name != "missing" {
				path = filepath.Join(dir, name+".jsonl")
				if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := measurement.LoadDecisions([]string{path}); err == nil {
				t.Fatal("unsafe telemetry must fail closed")
			}
		})
	}
}
