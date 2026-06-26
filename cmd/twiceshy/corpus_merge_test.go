// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCorpusMergeCheckRejectsCrossBranchDuplicateIDs(t *testing.T) {
	repo := initMergeRepo(t)
	writeMergeRecord(t, repo, "experience/2026/2759-base-record.md", "exp-2759")
	runMergeGit(t, repo, "add", ".")
	runMergeGit(t, repo, "commit", "-m", "base")
	base := mergeRev(t, repo, "HEAD")

	writeMergeRecord(t, repo, "experience/2026/2759-different-file.md", "exp-2759")
	runMergeGit(t, repo, "add", ".")
	runMergeGit(t, repo, "commit", "-m", "head")
	head := mergeRev(t, repo, "HEAD")

	var out bytes.Buffer
	err := run(context.Background(), []string{"corpus-merge-check", "-corpus", repo, "-base", base, "-head", head}, &out, func(string) string { return "" })
	if err == nil {
		t.Fatal("corpus-merge-check must reject a duplicate id introduced against base")
	}
	if !strings.Contains(err.Error(), "exp-2759") {
		t.Fatalf("error must name duplicate id, got %v", err)
	}
}

func TestRunCorpusPRPathsRejectsFixedRunsFiles(t *testing.T) {
	repo := initMergeRepo(t)
	writeMergeRecord(t, repo, "experience/2026/0001-base.md", "exp-0001")
	runMergeGit(t, repo, "add", ".")
	runMergeGit(t, repo, "commit", "-m", "base")
	base := mergeRev(t, repo, "HEAD")

	writeMergeFile(t, repo, "runs/promote.holds.json", "{}\n")
	runMergeGit(t, repo, "add", ".")
	runMergeGit(t, repo, "commit", "-m", "bad path")
	head := mergeRev(t, repo, "HEAD")

	var out bytes.Buffer
	err := run(context.Background(), []string{"corpus-pr-paths", "-corpus", repo, "-base", base, "-head", head}, &out, func(string) string { return "" })
	if err == nil {
		t.Fatal("corpus-pr-paths must reject fixed runs files")
	}
	if !strings.Contains(err.Error(), "runs/promote.holds.json") {
		t.Fatalf("error must name rejected path, got %v", err)
	}
}

func initMergeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runMergeGit(t, dir, "init", "-q")
	runMergeGit(t, dir, "config", "user.email", "test@example.com")
	runMergeGit(t, dir, "config", "user.name", "Test User")
	return dir
}

func runMergeGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func mergeRev(t *testing.T, dir, ref string) string {
	t.Helper()
	return strings.TrimSpace(runMergeGit(t, dir, "rev-parse", ref))
}

func writeMergeRecord(t *testing.T, root, rel, id string) {
	t.Helper()
	writeMergeFile(t, root, rel, `---
schema_version: 1
id: `+id+`
kind: trap
status: validated
title: "A valid record"
symptom:
  summary: "something observable went wrong"
  error_signatures: ["boom"]
applies_to:
  - ecosystem: Go
    package: example.com/mod
resolution:
  root_cause: "a cause"
  fix: "a fix"
guard: { repro: null, guarding_test: "TestThing" }
provenance:
  source: { author: "test", session: null, pr: null }
  recorded_at: 2026-06-26
  validated_at: 2026-06-26
  valid: { from: 2026-06-26, until: null }
  superseded_by: null
---

Narrative body.
`)
}

func writeMergeFile(t *testing.T, root, rel, data string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
