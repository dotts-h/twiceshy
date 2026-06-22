// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/spool"
)

// intake-issues drains the report_issue spool (#0066) into docs/issues/, mirroring
// intake-reports (#0042). Numbering reuses the canonical new-issue.sh allocator so
// the Go path and the human path never drift on issue numbers (exp-0743's stale-id
// lesson applied to issue numbers) — these tests therefore drive the REAL script.
func TestIntakeIssues_MaterializesQueueIntoDocsIssues(t *testing.T) {
	repo := setupIssuesRepo(t)
	queue := filepath.Join(t.TempDir(), "queue")
	for _, iss := range []spool.Issue{
		{Title: "Flaky retry on cold cache", Description: "The importer retries forever when the cache is cold.", Category: "bug", Author: "agent-7", ReportedAt: "2026-06-22T12:00:00Z"},
		{Title: "Add a dry-run to ingest", Description: "Would help preview imports.", Category: "feature", RelatedRecordID: "exp-0042", Author: "agent-9", ReportedAt: "2026-06-22T12:00:01Z"},
	} {
		if _, err := spool.EnqueueIssue(queue, iss); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	if err := runIntakeIssues([]string{"-repo", repo, "-queue", queue}, &buf); err != nil {
		t.Fatalf("runIntakeIssues: %v", err)
	}

	issuesDir := filepath.Join(repo, "docs", "issues")
	// Two new files beyond the seed (0001) — the allocator advanced to 0002, 0003.
	matches, _ := filepath.Glob(filepath.Join(issuesDir, "000[23]-*.md"))
	if len(matches) != 2 {
		t.Fatalf("want 2 materialized issues (0002,0003), got %d: %v", len(matches), matches)
	}
	var all strings.Builder
	for _, m := range matches {
		b, _ := os.ReadFile(m)
		all.Write(b)
	}
	body := all.String()
	for _, want := range []string{
		"The importer retries forever when the cache is cold.", // descriptions land in Summary
		"Would help preview imports.",
		"bug", "feature", // categories recorded
		"agent-7", "agent-9", // authors recorded
		"exp-0042",     // related record carried through
		"report_issue", // provenance note
		"## Summary", "## Notes",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("materialized issues missing %q", want)
		}
	}
	// INDEX rows appended for both (the script's append, not a second writer).
	idx, _ := os.ReadFile(filepath.Join(issuesDir, "INDEX.md"))
	if !strings.Contains(string(idx), "[0002]") || !strings.Contains(string(idx), "[0003]") {
		t.Errorf("INDEX.md missing rows for 0002/0003:\n%s", idx)
	}
	// Queue fully drained — nothing left to re-process.
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Fatalf("queue must be drained after intake; %d left", len(paths))
	}
}

func TestIntakeIssues_RequiresQueueFlag(t *testing.T) {
	var buf bytes.Buffer
	err := runIntakeIssues([]string{"-repo", "."}, &buf)
	if err == nil || !strings.Contains(err.Error(), "-queue") {
		t.Fatalf("intake-issues without -queue must fail clearly; got %v", err)
	}
}

// A spooled issue whose normalized title already exists in INDEX.md is skipped
// (case/spacing/punctuation-insensitive), so a re-submission never duplicates an
// open issue. A genuinely new sibling still materializes.
func TestIntakeIssues_SkipsDuplicateTitle(t *testing.T) {
	repo := setupIssuesRepo(t)
	queue := filepath.Join(t.TempDir(), "queue")
	// Near-duplicate of the seed "Existing known trap" (case/spacing/punctuation differ).
	if _, err := spool.EnqueueIssue(queue, spool.Issue{Title: "existing   known trap!", Description: "dup", Category: "bug", Author: "a", ReportedAt: "2026-06-22T12:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := spool.EnqueueIssue(queue, spool.Issue{Title: "A genuinely new thing", Description: "fresh", Category: "bug", Author: "a", ReportedAt: "2026-06-22T12:00:01Z"}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runIntakeIssues([]string{"-repo", repo, "-queue", queue}, &buf); err != nil {
		t.Fatalf("runIntakeIssues: %v", err)
	}

	issuesDir := filepath.Join(repo, "docs", "issues")
	if m, _ := filepath.Glob(filepath.Join(issuesDir, "0002-*.md")); len(m) != 1 {
		t.Fatalf("the new issue should materialize as 0002; got %v", m)
	}
	// The duplicate must NOT have allocated a number / written a file.
	if extra, _ := filepath.Glob(filepath.Join(issuesDir, "0003-*.md")); len(extra) != 0 {
		t.Fatalf("duplicate title must not materialize; got %v", extra)
	}
	if !strings.Contains(strings.ToLower(buf.String()), "duplicate") {
		t.Errorf("the skip should be reported as a duplicate; got %q", buf.String())
	}
	// Both entries drain (dup removed, new materialized) — neither wedges the queue.
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Fatalf("queue must be drained; %d left", len(paths))
	}
}

// A malformed queue entry (unreadable, or missing the title intake must allocate
// against) is logged and removed so it never wedges a scheduled drain; a valid
// sibling still materializes. Mirrors intake-reports' malformed handling.
func TestIntakeIssues_SkipsMalformedEntry(t *testing.T) {
	repo := setupIssuesRepo(t)
	queue := filepath.Join(t.TempDir(), "queue")
	if err := os.MkdirAll(queue, 0o755); err != nil {
		t.Fatal(err)
	}
	// Unreadable: not JSON at all.
	if err := os.WriteFile(filepath.Join(queue, "2026-06-22T00-00-00Z-garbage.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Titleless: intake cannot allocate a docs/issues entry for it.
	if _, err := spool.EnqueueIssue(queue, spool.Issue{Title: "   ", Description: "x", Category: "bug", Author: "a", ReportedAt: "2026-06-22T12:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := spool.EnqueueIssue(queue, spool.Issue{Title: "A real new issue", Description: "fix it", Category: "bug", Author: "a", ReportedAt: "2026-06-22T12:00:01Z"}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runIntakeIssues([]string{"-repo", repo, "-queue", queue}, &buf); err != nil {
		t.Fatalf("runIntakeIssues: %v", err)
	}
	if m, _ := filepath.Glob(filepath.Join(repo, "docs", "issues", "0002-*.md")); len(m) != 1 {
		t.Fatalf("the valid sibling should materialize as 0002; got %v", m)
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Fatalf("malformed + valid entries must all drain; %d left", len(paths))
	}
	if !strings.Contains(buf.String(), "skip") {
		t.Errorf("malformed entries should be reported as skipped; got %q", buf.String())
	}
}

// setupIssuesRepo builds a throwaway git repo carrying the REAL new-issue.sh and a
// seeded docs/issues/, so intake-issues exercises the same allocator the human path
// uses (the #0075 "no second allocator" requirement) rather than a test double.
func setupIssuesRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	// new-issue.sh resolves the repo root via `git rev-parse --show-toplevel`.
	if out, err := runGit(repo, "init"); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "new-issue.sh"))
	if err != nil {
		t.Fatalf("read new-issue.sh: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "scripts", "new-issue.sh"), script, 0o755); err != nil {
		t.Fatal(err)
	}
	issuesDir := filepath.Join(repo, "docs", "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// One seeded issue so numbering starts at 0002 and the dedup path has a title to
	// collide against.
	seed := "---\nid: 0001\ntitle: Existing known trap\nstatus: open\nseverity: medium\ngroup:\ndepends_on: []\nforgejo:\nlinks:\n  adr:\n  prs: []\n  issues: []\n  regression:\nassets: []\n---\n\n## Summary\nseed\n"
	if err := os.WriteFile(filepath.Join(issuesDir, "0001-existing-trap.md"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	index := "# Issues index\n\n## Issues\n\n| id | title | status | severity | group | links |\n|----|-------|--------|----------|-------|-------|\n| [0001](0001-existing-trap.md) | Existing known trap | open | medium | — | |\n"
	if err := os.WriteFile(filepath.Join(issuesDir, "INDEX.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
