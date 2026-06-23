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

// eval -push runs the push-precision eval over the real corpus: off-domain prompts
// must inject nothing (precision) and genuine traps must surface (recall). End-to-end
// guard for the CLI path the push channel is gated on. Both metrics are corpus-relative,
// so this needs the live corpus (now twiceshy-corpus, ADR-0021) and is livecorpus-tagged
// — run it with `make test-livecorpus` against a corpus checkout placed at ../..; it
// skips cleanly when none is present.
func TestRunEvalPush(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := os.Stat(filepath.Join(root, "experience")); err != nil {
		t.Skip("live corpus not present at ../.. (decoupled to twiceshy-corpus, ADR-0021); skipping push eval")
	}
	var out bytes.Buffer
	err := run(context.Background(), []string{
		"eval", "-push", "-corpus", root, "-db", filepath.Join(t.TempDir(), "pe.db"),
	}, &out, noEnv)
	if err != nil {
		t.Fatalf("eval -push: %v\n%s", err, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "precision: 100.0%") {
		t.Errorf("want precision 100%%, got:\n%s", s)
	}
	if !strings.Contains(s, "recall:    100.0%") {
		t.Errorf("want recall 100%%, got:\n%s", s)
	}
}
