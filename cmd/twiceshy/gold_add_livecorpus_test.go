//go:build livecorpus

// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The committed advisory-gold.yaml (#0074) must be exactly what `gold-add
// -advisory-audit` produces from the real audit + corpus — a golden test that both
// exercises the bulk generator end-to-end and catches the embed drifting from its
// source (regenerate and commit if this fails). BuildAdvisoryGold emits only the 85
// audited ids, so corpus growth from scheduled imports does not perturb it.
//
// It needs the live corpus (the 85 advisory records the audit references), which is
// now the twiceshy-corpus data product (ADR-0021), so it is livecorpus-tagged: run
// it with `make test-livecorpus` against a corpus checkout placed at ../.. — it
// skips cleanly when no corpus is present.
func TestGoldAddAdvisory_RegenerationMatchesCommitted(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "experience")); err != nil {
		t.Skip("live corpus not present at ../.. (decoupled to twiceshy-corpus, ADR-0021); " +
			"place a corpus checkout there to run the advisory-gold regeneration guard")
	}
	auditPath := filepath.Join(root, "runs", "sonnet-advisory-audit.json")
	goldPath := filepath.Join(t.TempDir(), "advisory-gold.yaml")

	var out bytes.Buffer
	if err := runGoldAdd(context.Background(), []string{
		"-corpus", root, "-advisory-audit", auditPath, "-gold-file", goldPath,
	}, &out); err != nil {
		t.Fatalf("runGoldAdd -advisory-audit: %v", err)
	}
	if !strings.Contains(out.String(), "wrote 85 advisory gold case") {
		t.Errorf("unexpected summary: %q", out.String())
	}

	got, err := os.ReadFile(goldPath)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(filepath.Join(root, "internal", "judgeeval", "advisory-gold.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, committed) {
		t.Error("regenerated advisory-gold.yaml differs from the committed embed — " +
			"re-run `twiceshy gold-add -advisory-audit runs/sonnet-advisory-audit.json` and commit the result")
	}
}
