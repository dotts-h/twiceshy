// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/measurement"
)

func TestPilotReportJSONAndCSV(t *testing.T) {
	d := t.TempDir()
	telemetry := filepath.Join(d, "decisions.jsonl")
	cohorts := filepath.Join(d, "cohorts.csv")
	outcomes := filepath.Join(d, "outcomes.jsonl")
	session := "0123456789abcdef0123456789abcdef"
	must := func(p, s string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	must(telemetry, `{"ts":"2026-07-08T01:00:00Z","channel":"push","query_hash":"q","session":"`+session+`","served":[{"id":"exp-0001","score":1}],"count":1,"trigger":"error","query_text":"must-not-leak"}`+"\n")
	must(cohorts, "team,session_hash\nteam-a,"+session+"\n")
	must(outcomes, `{"ts":"2026-07-08T02:00:00Z","session_hash":"`+session+`","record_id":"exp-0001","used":true,"confirmed":true}`+"\n")
	args := []string{"-telemetry", telemetry, "-cohorts", cohorts, "-outcomes", outcomes, "-baseline-start", "2026-07-01T00:00:00Z", "-baseline-end", "2026-07-02T00:00:00Z", "-treatment-start", "2026-07-08T00:00:00Z", "-treatment-end", "2026-07-09T00:00:00Z"}
	var out bytes.Buffer
	if err := runPilotReport(append(args, "-format", "json"), &out); err != nil {
		t.Fatal(err)
	}
	var rep measurement.Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Treatment.Metrics.Used != 1 {
		t.Fatalf("report=%+v", rep)
	}
	if strings.Contains(out.String(), "must-not-leak") {
		t.Fatal("raw query leaked")
	}
	first := out.String()
	out.Reset()
	if err := runPilotReport(append(args, "-format", "json"), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != first {
		t.Fatal("JSON report is not deterministic")
	}
	out.Reset()
	if err := runPilotReport(append(args, "-format", "csv"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "scope,arm,team,record_id,") {
		t.Fatalf("CSV=%s", out.String())
	}
}
