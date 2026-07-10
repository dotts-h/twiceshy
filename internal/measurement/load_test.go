// SPDX-License-Identifier: AGPL-3.0-only

package measurement_test

import (
	"os"
	"path/filepath"
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
