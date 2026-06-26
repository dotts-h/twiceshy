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

// dry-run lists the quarantined candidates and writes nothing — and crucially
// it never constructs the broker, so it runs without Docker/runsc.
func TestRunDraftDryRunListsCandidatesAndWritesNothing(t *testing.T) {
	dir := tempCorpus(t)
	// Seed the corpus with quarantined records via the existing importer.
	if err := run(context.Background(), []string{"ingest", "go", "-corpus", dir,
		"-db", filepath.Join(t.TempDir(), "ix.db")}, &bytes.Buffer{}, noEnv); err != nil {
		t.Fatalf("ingest go: %v", err)
	}

	var out bytes.Buffer
	if err := run(context.Background(), []string{"draft", "-corpus", dir, "-dry-run"}, &out, noEnv); err != nil {
		t.Fatalf("draft -dry-run: %v", err)
	}
	if !strings.Contains(out.String(), "candidate") || !strings.Contains(out.String(), "dry-run") {
		t.Errorf("dry-run should list candidates; output = %q", out.String())
	}
	// No repro directories were created.
	if entries, _ := os.ReadDir(filepath.Join(dir, "experience", "repro")); len(entries) != 0 {
		t.Errorf("dry-run wrote repro artifacts: %v", entries)
	}
	// Records are untouched (no guard attached).
	recs, err := record.LoadCorpus(dir)
	if err != nil {
		t.Fatalf("LoadCorpus after dry-run: %v", err)
	}
	for _, r := range recs {
		if r.Guard != nil && len(r.Guard.Repros) > 0 {
			t.Errorf("dry-run must not attach a repro to %s", r.ID)
		}
	}
}

func TestRunDraftBadFlag(t *testing.T) {
	if err := run(context.Background(), []string{"draft", "-nope"}, &bytes.Buffer{}, noEnv); err == nil {
		t.Error("an unknown flag must error")
	}
}

func TestRunDraftRejectsInvalidCorpus(t *testing.T) {
	if err := run(context.Background(), []string{"draft", "-corpus", t.TempDir(), "-dry-run"}, &bytes.Buffer{}, noEnv); err == nil {
		t.Error("a corpus without experience/ must fail")
	}
}

// TestDraftersFrom covers the env-gated drafter chain (#0026 slice 3):
// deterministic-only by default, with the model drafter appended (model id
// defaulted, then honored) when TWICESHY_DRAFTER_URL is configured.
func TestDraftersFrom(t *testing.T) {
	// No model endpoint → deterministic-only (a bare checkout is unchanged).
	ds := draftersFrom(noEnv)
	if len(ds) != 1 {
		t.Fatalf("no env → deterministic drafter only; got %d", len(ds))
	}
	if ds[0].Name() != "go-deprecation-template" {
		t.Errorf("first drafter should be the deterministic template; got %q", ds[0].Name())
	}

	// TWICESHY_DRAFTER_URL set → model drafter appended; model id defaults.
	env := map[string]string{"TWICESHY_DRAFTER_URL": "http://localhost:11434"}
	ds = draftersFrom(func(k string) string { return env[k] })
	if len(ds) != 2 {
		t.Fatalf("with drafter url → deterministic + model; got %d", len(ds))
	}
	if got := ds[1].Name(); got != "model-drafter(qwen2.5-coder:14b)" {
		t.Errorf("model drafter should default the model id; got %q", got)
	}

	// An explicit model id is honored.
	env["TWICESHY_DRAFTER_MODEL"] = "custom:7b"
	ds = draftersFrom(func(k string) string { return env[k] })
	if got := ds[1].Name(); got != "model-drafter(custom:7b)" {
		t.Errorf("explicit model id should be used; got %q", got)
	}
}
