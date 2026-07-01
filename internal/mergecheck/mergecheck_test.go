// SPDX-License-Identifier: AGPL-3.0-only

package mergecheck_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/mergecheck"
)

func TestCorpusMergeCheckRejectsCrossBranchDuplicateIDs(t *testing.T) {
	repo := initRepo(t)
	writeRecord(t, repo, "experience/2026/2759-base-record.md", "exp-2759")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	base := rev(t, repo, "HEAD")

	writeRecord(t, repo, "experience/2026/2759-different-file.md", "exp-2759")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "head")
	head := rev(t, repo, "HEAD")

	err := mergecheck.CorpusMergeCheck(context.Background(), mergecheck.MergeParams{
		Corpus: repo,
		Base:   base,
		Head:   head,
	})
	if err == nil {
		t.Fatal("duplicate id introduced against base must fail")
	}
	msg := err.Error()
	for _, want := range []string{"exp-2759", "2759-different-file.md", "2759-base-record.md"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error must mention %q; got %v", want, err)
		}
	}
}

func TestCorpusMergeCheckRejectsIntraPRDuplicateIDs(t *testing.T) {
	repo := initRepo(t)
	writeRecord(t, repo, "experience/2026/2758-base-record.md", "exp-2758")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	base := rev(t, repo, "HEAD")

	writeRecord(t, repo, "experience/2026/2759-one.md", "exp-2759")
	writeRecord(t, repo, "experience/2026/2759-two.md", "exp-2759")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "head")
	head := rev(t, repo, "HEAD")

	err := mergecheck.CorpusMergeCheck(context.Background(), mergecheck.MergeParams{
		Corpus: repo,
		Base:   base,
		Head:   head,
	})
	if err == nil {
		t.Fatal("two introduced files with the same id must fail")
	}
	msg := err.Error()
	for _, want := range []string{"exp-2759", "2759-one.md", "2759-two.md"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error must mention %q; got %v", want, err)
		}
	}
}

func TestCorpusMergeCheckPassesForFreshIDs(t *testing.T) {
	repo := initRepo(t)
	writeRecord(t, repo, "experience/2026/2758-base-record.md", "exp-2758")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	base := rev(t, repo, "HEAD")

	writeRecord(t, repo, "experience/2026/2759-one.md", "exp-2759")
	writeRecord(t, repo, "experience/2026/2760-two.md", "exp-2760")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "head")
	head := rev(t, repo, "HEAD")

	err := mergecheck.CorpusMergeCheck(context.Background(), mergecheck.MergeParams{
		Corpus: repo,
		Base:   base,
		Head:   head,
	})
	if err != nil {
		t.Fatalf("fresh ids must pass: %v", err)
	}
}

// A record introduced (or already on base) that fails to parse must surface a
// wrapped, path-named error — not a panic or a silently skipped check.
func TestCorpusMergeCheckRejectsUnparsableIntroducedRecord(t *testing.T) {
	repo := initRepo(t)
	writeRecord(t, repo, "experience/2026/2758-base-record.md", "exp-2758")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	base := rev(t, repo, "HEAD")

	writeFile(t, repo, "experience/2026/2759-broken.md", "not a valid record at all\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "head")
	head := rev(t, repo, "HEAD")

	err := mergecheck.CorpusMergeCheck(context.Background(), mergecheck.MergeParams{
		Corpus: repo,
		Base:   base,
		Head:   head,
	})
	if err == nil {
		t.Fatal("an unparsable introduced record must fail the check")
	}
	if !strings.Contains(err.Error(), "2759-broken.md") {
		t.Fatalf("error must name the unparsable path, got %v", err)
	}
}

// Every required MergeParams field is checked independently — a caller that
// forgets to wire one up (e.g. leaves Base empty) must get a clear error, not a
// git invocation against an empty/implicit ref.
func TestMergeParams_RequiresAllFields(t *testing.T) {
	full := mergecheck.MergeParams{Corpus: "/tmp/x", Base: "main", Head: "HEAD"}
	cases := []struct {
		name   string
		params mergecheck.MergeParams
		want   string
	}{
		{"missing corpus", mergecheck.MergeParams{Base: full.Base, Head: full.Head}, "corpus is required"},
		{"missing base", mergecheck.MergeParams{Corpus: full.Corpus, Head: full.Head}, "base ref is required"},
		{"missing head", mergecheck.MergeParams{Corpus: full.Corpus, Base: full.Base}, "head ref is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := mergecheck.CorpusPRPaths(context.Background(), tc.params)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CorpusPRPaths(%+v) = %v, want it to mention %q", tc.params, err, tc.want)
			}
		})
	}
}

func TestCorpusPRPathsRejectsFixedRunsFiles(t *testing.T) {
	repo := initRepo(t)
	writeRecord(t, repo, "experience/2026/0001-base.md", "exp-0001")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	base := rev(t, repo, "HEAD")

	writeFile(t, repo, "runs/promote.holds.json", "{}\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "bad path")
	head := rev(t, repo, "HEAD")

	err := mergecheck.CorpusPRPaths(context.Background(), mergecheck.MergeParams{
		Corpus: repo,
		Base:   base,
		Head:   head,
	})
	if err == nil {
		t.Fatal("fixed runs file must be rejected")
	}
	if !strings.Contains(err.Error(), "runs/promote.holds.json") {
		t.Fatalf("error must name rejected path, got %v", err)
	}
}

func TestCorpusPRPathsPassesAllowedPaths(t *testing.T) {
	repo := initRepo(t)
	writeRecord(t, repo, "experience/2026/0001-base.md", "exp-0001")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	base := rev(t, repo, "HEAD")

	writeRecord(t, repo, "experience/2026/0002-next.md", "exp-0002")
	writeFile(t, repo, "experience/repro/0002.sh", "#!/bin/sh\nexit 0\n")
	writeFile(t, repo, "runs/run-20260626-promote.json", "{}\n")
	writeFile(t, repo, "runs/run-20260626-adapt.json", "{}\n")
	// Ordinary repo-maintenance files must NOT be blocked (denylist, not allowlist):
	// e.g. the ADR-0027 .gitignore PR itself would have been rejected by an allowlist.
	writeFile(t, repo, ".gitignore", "runs/*.journal.json\n")
	writeFile(t, repo, "README.md", "# corpus\n")
	writeFile(t, repo, ".forgejo/workflows/validate.yml", "name: corpus-ci\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "allowed paths")
	head := rev(t, repo, "HEAD")

	err := mergecheck.CorpusPRPaths(context.Background(), mergecheck.MergeParams{
		Corpus: repo,
		Base:   base,
		Head:   head,
	})
	if err != nil {
		t.Fatalf("allowed PR paths must pass: %v", err)
	}
}

// The denylist must reject every fixed operational file, not just the holds ledger.
func TestCorpusPRPathsRejectsAllOperationalFiles(t *testing.T) {
	for _, bad := range []string{
		"runs/promote.journal.json",
		"runs/adapt.journal.json",
		"runs/promote.holds.json",
		"corpus.db",
		"runs/.twiceshy-loop.lock",
	} {
		t.Run(bad, func(t *testing.T) {
			repo := initRepo(t)
			writeRecord(t, repo, "experience/2026/0001-base.md", "exp-0001")
			git(t, repo, "add", ".")
			git(t, repo, "commit", "-m", "base")
			base := rev(t, repo, "HEAD")

			writeFile(t, repo, bad, "x\n")
			git(t, repo, "add", ".")
			git(t, repo, "commit", "-m", "bad")
			head := rev(t, repo, "HEAD")

			err := mergecheck.CorpusPRPaths(context.Background(), mergecheck.MergeParams{Corpus: repo, Base: base, Head: head})
			if err == nil {
				t.Fatalf("%s must be rejected", bad)
			}
			if !strings.Contains(err.Error(), bad) {
				t.Fatalf("error must name %q, got %v", bad, err)
			}
		})
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test User")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func rev(t *testing.T, dir, ref string) string {
	t.Helper()
	return strings.TrimSpace(git(t, dir, "rev-parse", ref))
}

func writeRecord(t *testing.T, root, rel, id string) {
	t.Helper()
	writeFile(t, root, rel, `---
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

func writeFile(t *testing.T, root, rel, data string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
