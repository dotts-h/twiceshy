// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/agenteval"
)

func TestRunProspectBadFlag(t *testing.T) {
	if err := run(context.Background(), []string{"prospect", "-nope"}, &bytes.Buffer{}, noEnv); err == nil {
		t.Error("an unknown flag must error")
	}
}

func TestRunProspectRequiresCorpus(t *testing.T) {
	if err := run(context.Background(), []string{"prospect"}, &bytes.Buffer{}, noEnv); err == nil {
		t.Error("missing -corpus must error")
	}
}

// A corpus without experience/ fails at LoadCorpus, before the broker/model are
// ever touched — this must not require docker or a model endpoint to test.
func TestRunProspectRejectsInvalidCorpus(t *testing.T) {
	err := run(context.Background(), []string{"prospect", "-corpus", t.TempDir()}, &bytes.Buffer{}, noEnv)
	if err == nil {
		t.Error("a corpus without experience/ must fail")
	}
}

// writeProspectReport + printProspectSummary are the report-writing half of the
// CLI, tested directly (no docker/model endpoint needed) — the fields a
// downstream reader depends on must round-trip.
func TestWriteProspectReport_ExpectedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "prospect-report.json")
	rep := agenteval.ProspectReport{
		Scanned: 5, Eligible: 3, Drafted: 2,
		Skipped:    map[string]int{"ineligible": 2, "leak": 1},
		OffAvoided: []string{"exp-0001"},
		ModelHard: []agenteval.ProspectCase{
			{TrapID: "exp-0002", Prompt: "p", VerifyID: "gobuild", OnAvoided: true, TokensOff: 10, TokensOn: 20},
			{TrapID: "exp-0003", Prompt: "p2", VerifyID: "tsc", OnAvoided: false, TokensOff: 15, TokensOn: 25},
		},
	}
	if err := writeProspectReport(path, rep); err != nil {
		t.Fatalf("writeProspectReport: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written report: %v", err)
	}
	var got agenteval.ProspectReport
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshaling written report: %v", err)
	}
	if got.Scanned != 5 || got.Eligible != 3 || got.Drafted != 2 {
		t.Errorf("scanned/eligible/drafted = %d/%d/%d, want 5/3/2", got.Scanned, got.Eligible, got.Drafted)
	}
	if got.Skipped["ineligible"] != 2 || got.Skipped["leak"] != 1 {
		t.Errorf("Skipped = %v, want ineligible:2 leak:1", got.Skipped)
	}
	if len(got.OffAvoided) != 1 || got.OffAvoided[0] != "exp-0001" {
		t.Errorf("OffAvoided = %v", got.OffAvoided)
	}
	if len(got.ModelHard) != 2 {
		t.Fatalf("ModelHard len = %d, want 2", len(got.ModelHard))
	}
	if got.ModelHard[0].TrapID != "exp-0002" || !got.ModelHard[0].OnAvoided {
		t.Errorf("ModelHard[0] = %+v", got.ModelHard[0])
	}
	if got.ModelHard[1].TrapID != "exp-0003" || got.ModelHard[1].OnAvoided {
		t.Errorf("ModelHard[1] = %+v", got.ModelHard[1])
	}

	var out bytes.Buffer
	printProspectSummary(&out, rep, path)
	summary := out.String()
	for _, want := range []string{"scanned 5", "eligible 3", "drafted 2", "model-hard: 2", "on-also-fails: 1", path} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing %q; got:\n%s", want, summary)
		}
	}
}

func TestOnAlsoFailsCount(t *testing.T) {
	rep := agenteval.ProspectReport{ModelHard: []agenteval.ProspectCase{
		{TrapID: "exp-0001", OnAvoided: true},
		{TrapID: "exp-0002", OnAvoided: false},
		{TrapID: "exp-0003", OnAvoided: false},
	}}
	if got := onAlsoFailsCount(rep); got != 2 {
		t.Errorf("onAlsoFailsCount = %d, want 2", got)
	}
}

func TestDefaultProspectReportPath(t *testing.T) {
	p := defaultProspectReportPath()
	if !strings.HasPrefix(filepath.ToSlash(p), "runs/prospect-") || !strings.HasSuffix(p, ".json") {
		t.Errorf("defaultProspectReportPath() = %q, want runs/prospect-<ts>.json", p)
	}
}
