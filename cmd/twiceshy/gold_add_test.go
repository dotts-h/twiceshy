// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

func TestGoldAdd_PrintsStanza(t *testing.T) {
	dir := tempCorpus(t)
	reproPath := "experience/repro/exp-0058/repro.sh"
	reproContent := "#!/bin/sh\necho GOLD_ADD_REPRO_CONTENT\n"
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(reproPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, reproPath), []byte(reproContent), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &record.Record{
		SchemaVersion: 1,
		ID:            "exp-0058",
		Kind:          "convention",
		Status:        "quarantined",
		Title:         "Gold-add test record",
		Symptom:       &record.Symptom{Summary: "test symptom"},
		Resolution:    &record.Resolution{RootCause: "test", Fix: "test fix"},
		AppliesTo:     []record.AppliesTo{{Ecosystem: "Go", Package: "fmt"}},
		Guard: &record.Guard{
			Repros: []record.Repro{{
				Path: reproPath, Kind: "positive", Label: "test repro",
			}},
		},
		Provenance: record.Provenance{
			Source:     record.Source{Author: "test"},
			RecordedAt: "2026-06-19",
			Valid:      record.Validity{From: "2026-06-19"},
		},
		Body: "Narrative for gold-add test.",
		Path: "experience/2026/0058-gold-add-test.md",
	}
	writeFixture(t, dir, rec)

	var out bytes.Buffer
	err := runGoldAdd(context.Background(), []string{
		"-corpus", dir,
		"-record", rec.Path,
		"-id", "G99",
		"-mode", "license",
		"-checks", "license",
		"-rationale", "audit miss: judge approved encumbered content",
	}, &out)
	if err != nil {
		t.Fatalf("runGoldAdd: %v", err)
	}
	s := out.String()
	for _, want := range []string{"id: G99", "mode: license", "GOLD_ADD_REPRO_CONTENT"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
}

func TestGoldAdd_RequiresFlags(t *testing.T) {
	if err := runGoldAdd(context.Background(), []string{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error without required flags")
	}
}

func TestGoldAdd_RecordWithoutReproErrors(t *testing.T) {
	dir := tempCorpus(t)
	rec := packFixture("0059", "quarantined", "MIT", "")
	rec.Symptom = &record.Symptom{Summary: "s"}
	rec.Resolution = &record.Resolution{RootCause: "r", Fix: "f"}
	writeFixture(t, dir, rec)

	err := runGoldAdd(context.Background(), []string{
		"-corpus", dir, "-record", rec.Path,
		"-id", "G59", "-mode", "license", "-checks", "license", "-rationale", "x",
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "at least one repro") {
		t.Fatalf("err = %v, want repro required", err)
	}
}

func TestGoldAdd_AppendWritesGoldFile(t *testing.T) {
	dir := tempCorpus(t)
	reproPath := "experience/repro/exp-0060/repro.sh"
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(reproPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, reproPath), []byte("echo append\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := packFixture("0060", "quarantined", "MIT", "")
	rec.Symptom = &record.Symptom{Summary: "s"}
	rec.Resolution = &record.Resolution{RootCause: "r", Fix: "f"}
	rec.Guard = &record.Guard{Repros: []record.Repro{{Path: reproPath, Kind: "positive"}}}
	writeFixture(t, dir, rec)

	goldPath := filepath.Join(t.TempDir(), "gold.yaml")
	if err := os.WriteFile(goldPath, []byte("cases:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runGoldAdd(context.Background(), []string{
		"-corpus", dir, "-record", rec.Path,
		"-id", "G60", "-mode", "scope", "-checks", "scope", "-rationale", "audit miss",
		"-gold-file", goldPath, "-append",
	}, &out); err != nil {
		t.Fatalf("runGoldAdd: %v", err)
	}
	body, err := os.ReadFile(goldPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "id: G60") {
		t.Fatalf("gold file = %q", body)
	}
}
