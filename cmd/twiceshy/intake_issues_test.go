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

// An agent-controlled title containing newlines, YAML metacharacters and a pipe must
// not inject frontmatter keys, split the INDEX table row, or corrupt the filename — it
// is reduced to a single safe line. The server's no-queue path %q-quotes the title for
// exactly this reason; the drainer (which routes the title through the unquoted
// new-issue.sh) must sanitize it instead. (Reviewer-found HIGH; #0075.)
func TestIntakeIssues_SanitizesUnsafeTitle(t *testing.T) {
	repo := setupIssuesRepo(t)
	queue := filepath.Join(t.TempDir(), "queue")
	if _, err := spool.EnqueueIssue(queue, spool.Issue{
		Title:       "Boom\nstatus: closed\nseverity: critical | pwn",
		Description: "x", Category: "bug", Author: "a", ReportedAt: "2026-06-22T12:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runIntakeIssues([]string{"-repo", repo, "-queue", queue}, &buf); err != nil {
		t.Fatalf("runIntakeIssues: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(repo, "docs", "issues", "0002-*.md"))
	if len(matches) != 1 {
		t.Fatalf("want one materialized issue 0002, got %v", matches)
	}
	fm := frontmatter(t, matches[0])
	// Exactly one of each templated key — the malicious newlines did NOT add lines.
	if n := countPrefixLines(fm, "status:"); n != 1 {
		t.Fatalf("status injected: %d status lines in frontmatter:\n%s", n, fm)
	}
	if n := countPrefixLines(fm, "severity:"); n != 1 {
		t.Fatalf("severity injected: %d severity lines:\n%s", n, fm)
	}
	if n := countPrefixLines(fm, "title:"); n != 1 {
		t.Fatalf("title spans %d lines (newline injection):\n%s", n, fm)
	}
	if !strings.Contains(fm, "status: open") || !strings.Contains(fm, "severity: medium") {
		t.Errorf("templated status/severity overwritten by injection:\n%s", fm)
	}
	// The INDEX row for 0002 is a single, well-formed table row (6 cells → 7 pipes),
	// not split across physical lines by the title's newlines or broken by its pipe.
	idx, _ := os.ReadFile(filepath.Join(repo, "docs", "issues", "INDEX.md"))
	rows := 0
	for _, ln := range strings.Split(string(idx), "\n") {
		if strings.HasPrefix(ln, "| [0002]") {
			rows++
			if got := strings.Count(ln, "|"); got != 7 {
				t.Errorf("INDEX row for 0002 malformed (%d pipes, want 7): %q", got, ln)
			}
		}
	}
	if rows != 1 {
		t.Fatalf("want exactly one INDEX row for 0002, got %d", rows)
	}
}

// rollbackAllocation undoes a partial materialize so a failed body-write leaves no
// orphan file/INDEX row that would make the retry look like a duplicate and silently
// drop the captured issue (#0075, reviewer-found). Unrelated rows are preserved.
func TestRollbackAllocation_RemovesFileAndIndexRow(t *testing.T) {
	dir := t.TempDir()
	issuesDir := filepath.Join(dir, "docs", "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(issuesDir, "0099-orphan.md")
	if err := os.WriteFile(path, []byte("stub template"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(issuesDir, "INDEX.md")
	index := "| id | title | status | severity | group | links |\n|----|----|----|----|----|----|\n" +
		"| [0098](0098-keep.md) | Keep | open | medium | — | |\n" +
		"| [0099](0099-orphan.md) | Orphan | open | medium | — | |\n"
	if err := os.WriteFile(indexPath, []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}

	rollbackAllocation(path, indexPath, "0099")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("orphan file should be removed; stat err=%v", err)
	}
	idx, _ := os.ReadFile(indexPath)
	if strings.Contains(string(idx), "[0099]") {
		t.Errorf("INDEX row for 0099 should be stripped:\n%s", idx)
	}
	if !strings.Contains(string(idx), "[0098]") {
		t.Errorf("unrelated INDEX row 0098 must be preserved:\n%s", idx)
	}
}

// frontmatter returns the YAML frontmatter block (between the first two `---`) of a
// materialized issue file.
func frontmatter(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.HasPrefix(s, "---\n") {
		t.Fatalf("%s has no opening frontmatter:\n%s", path, s)
	}
	rest := s[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		t.Fatalf("%s has no closing frontmatter:\n%s", path, s)
	}
	return rest[:end]
}

func countPrefixLines(fm, prefix string) int {
	n := 0
	for _, ln := range strings.Split(fm, "\n") {
		if strings.HasPrefix(ln, prefix) {
			n++
		}
	}
	return n
}

// setupIssuesRepo builds a throwaway git repo carrying the REAL new-issue.sh and
// its issue-index generator plus a seeded docs/issues/, so intake-issues exercises
// the same allocator and derived-index path the human workflow uses rather than a
// test double.
func setupIssuesRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	// new-issue.sh resolves the repo root via `git rev-parse --show-toplevel`.
	if out, err := runGit(repo, "init"); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"new-issue.sh", "generate-issues-index.sh"} {
		script, err := os.ReadFile(filepath.Join("..", "..", "scripts", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(repo, "scripts", name), script, 0o755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
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

// A non-zero new-issue.sh aborts the whole unattended intake batch, so its stderr
// must be surfaced in the returned error, not reduced to "exit status N".
func TestRunNewIssue_SurfacesScriptStderr(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "failing.sh")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\necho 'BOOM_MARKER_42 not on a branch' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	_, err := runNewIssue(dir, script, "a title")
	if err == nil {
		t.Fatal("expected an error from a failing script")
	}
	if !strings.Contains(err.Error(), "BOOM_MARKER_42") {
		t.Errorf("error must surface the script stderr, got: %v", err)
	}
}
