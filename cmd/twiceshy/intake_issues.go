// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/dotts-h/twiceshy/internal/screen"
	"github.com/dotts-h/twiceshy/internal/spool"
)

// runIntakeIssues drains the report_issue queue (#0066, #0075): each spooled
// agent-submitted issue is materialized into docs/issues/ with a freshly allocated
// number, mirroring intake-reports (ADR-0013 §E1). Numbering, the INDEX append, and
// the file template come from the canonical scripts/new-issue.sh — never a second
// allocator (the exp-0743 stale-id lesson applies to issue numbers too) — and the
// drainer only fills the created file's body and screens the content. A spooled
// issue whose normalized title already exists is skipped (no duplicate). A malformed
// entry is logged and removed so it cannot wedge a scheduled drain; a partial
// materialize is rolled back so the entry is retried rather than silently lost.
// Issues land triage-flagged.
func runIntakeIssues(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("intake-issues", flag.ContinueOnError)
	repo := fs.String("repo", ".", "repo root containing docs/issues/ and scripts/new-issue.sh")
	queue := fs.String("queue", "", "issue queue directory written by `serve -issue-queue` (required)")
	script := fs.String("script", "", "path to new-issue.sh (default: <repo>/scripts/new-issue.sh)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *queue == "" {
		return errors.New("intake-issues requires -queue <dir> (the directory serve enqueues issues into)")
	}
	newIssue := *script
	if newIssue == "" {
		newIssue = filepath.Join(*repo, "scripts", "new-issue.sh")
	}
	indexPath := filepath.Join(*repo, "docs", "issues", "INDEX.md")

	// The repo root (new-issue.sh prints paths relative to it) is invariant across the
	// whole batch — resolve it once, not per issue.
	root, err := gitToplevel(*repo)
	if err != nil {
		return err
	}
	seen, err := existingIssueTitles(indexPath)
	if err != nil {
		return fmt.Errorf("reading issue index: %w", err)
	}
	files, err := spool.List(*queue)
	if err != nil {
		return fmt.Errorf("listing issue queue: %w", err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	intaken, dup, skipped := 0, 0, 0
	for _, f := range files {
		base := filepath.Base(f)
		iss, err := spool.ReadIssue(f)
		if err != nil || strings.TrimSpace(iss.Title) == "" {
			// Unreadable or titleless — cannot materialize; log + remove so it never
			// wedges a scheduled drain (mirrors intake-reports' malformed handling).
			_, _ = fmt.Fprintf(out, "  skip %s: unreadable or empty issue\n", base)
			_ = spool.Remove(f)
			skipped++
			continue
		}
		key := normalizeTitle(iss.Title)
		if seen[key] {
			_, _ = fmt.Fprintf(out, "  skip %s: duplicate title %q (already in docs/issues)\n", base, iss.Title)
			_ = spool.Remove(f)
			dup++
			continue
		}
		path, err := materializeIssue(*repo, root, newIssue, indexPath, iss, today)
		if err != nil {
			// Allocation/write failure is environmental — leave the entry queued for
			// retry rather than dropping a captured issue.
			return fmt.Errorf("materializing issue %q: %w", iss.Title, err)
		}
		seen[key] = true // within-batch dedup: a second copy queued before this drain
		_ = spool.Remove(f)
		intaken++
		_, _ = fmt.Fprintf(out, "  intake %s -> %s (%s)\n", base, filepath.Base(path), iss.Category)
	}
	_, _ = fmt.Fprintf(out, "intake-issues: materialized %d issue(s) into docs/issues/, %d duplicate, %d skipped\n", intaken, dup, skipped)
	return nil
}

// materializeIssue allocates the next issue number via the canonical new-issue.sh (so
// numbering and the INDEX row never drift from the human path) and fills the created
// file's body with the agent's submission. Everything that can fail without side
// effects is computed BEFORE the allocator runs, so the only fallible step after the
// script writes the file + INDEX row is the atomic body-write — which, on failure, is
// rolled back so the spooled issue re-materializes cleanly rather than being silently
// lost. It returns the created file's absolute path.
func materializeIssue(repo, root, script, indexPath string, iss spool.Issue, now string) (string, error) {
	// The title is agent-controlled: reduce it to a single safe line so it cannot
	// inject YAML frontmatter lines, split the INDEX table row, or smuggle newlines
	// into the filename. (The body is data, rendered below; the title goes through the
	// unquoted new-issue.sh frontmatter + INDEX row, so it must be sanitized.)
	safeTitle := sanitizeTitle(iss.Title)
	if safeTitle == "" {
		return "", errors.New("title is empty after sanitization")
	}
	flags := screen.Flags(screen.Scan(iss.Title, iss.Description))
	body := iss.RenderBody(now, flags)

	// Phase 1: allocate the number, append the INDEX row, write the template file. The
	// title is a single argv (no shell), so it cannot inject flags or shell syntax.
	rel, err := runNewIssue(repo, script, safeTitle)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, rel)

	// Phase 2: swap the empty template body for the issue's content. Any failure here
	// rolls back phase 1's file + INDEX row so the still-queued entry re-materializes
	// cleanly (otherwise its own orphaned INDEX row would make the retry look like a
	// duplicate and silently drop it).
	final, err := fillIssueBody(path, body)
	if err != nil {
		rollbackAllocation(path, indexPath, issueID(rel))
		return "", err
	}
	if err := writeFileAtomic(path, []byte(final), 0o644); err != nil {
		rollbackAllocation(path, indexPath, issueID(rel))
		return "", err
	}
	return path, nil
}

// runNewIssue invokes the canonical allocator and returns the created file's path
// relative to the repo root (the path new-issue.sh prints on stdout).
func runNewIssue(repo, script, title string) (string, error) {
	cmd := exec.Command("bash", script, title, "--severity", "medium")
	cmd.Dir = repo
	stdout, err := cmd.Output()
	if err != nil {
		// Surface the script's stderr (intake-issues is an unattended drainer that
		// aborts the whole batch on failure; "exit status 1" alone is undebuggable).
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("new-issue.sh: %w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("new-issue.sh: %w", err)
	}
	rel := strings.TrimSpace(string(stdout))
	if rel == "" {
		return "", errors.New("new-issue.sh printed no path")
	}
	return rel, nil
}

// fillIssueBody keeps the script-written frontmatter (the single source of the
// id/title/severity shape) and swaps the empty template body for the issue's content.
// The split is on the frontmatter delimiter; the title is single-line (sanitized) so it
// cannot contain a spurious delimiter that would hijack the split.
func fillIssueBody(path, body string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	front, _, ok := strings.Cut(string(content), "\n---\n")
	if !ok {
		return "", fmt.Errorf("created issue %s has no frontmatter delimiter", filepath.Base(path))
	}
	return front + "\n---\n\n" + body, nil
}

// rollbackAllocation undoes a partial materialize (best-effort): it removes the file
// new-issue.sh created and strips the INDEX row it appended, so the still-queued spool
// entry re-materializes cleanly on the next drain rather than colliding with its own
// orphaned row.
func rollbackAllocation(path, indexPath, id string) {
	_ = os.Remove(path)
	if id == "" {
		return
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return
	}
	prefix := "| [" + id + "]"
	var kept []string
	for _, ln := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(ln, prefix) {
			kept = append(kept, ln)
		}
	}
	_ = writeFileAtomic(indexPath, []byte(strings.Join(kept, "\n")), 0o644)
}

// issueID extracts the NNNN number new-issue.sh allocated from the created path.
func issueID(rel string) string {
	base := filepath.Base(rel)
	if len(base) >= 4 {
		return base[:4]
	}
	return ""
}

// sanitizeTitle reduces an agent-controlled title to a single safe line: every run of
// whitespace or control characters (including newlines and tabs) collapses to one
// space, and a pipe — which would break the docs/issues markdown table row
// new-issue.sh appends — becomes a slash. A one-line title cannot inject frontmatter
// keys, split the INDEX row, or smuggle newlines into the filename (the contract is a
// one-line summary).
func sanitizeTitle(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		switch {
		case r == '|':
			b.WriteByte('/')
			prevSpace = false
		case r < 0x20 || r == 0x7f || unicode.IsSpace(r):
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// existingIssueTitles reads the normalized titles already tracked in docs/issues so
// intake skips a re-submitted duplicate. It parses the INDEX table rows (offline,
// not the hot path); a missing index is an empty set, not an error.
func existingIssueTitles(indexPath string) (map[string]bool, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	titles := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		// An issue/epic row: | [NNNN](file.md) | <title> | ... |
		if !strings.HasPrefix(strings.TrimSpace(line), "| [") {
			continue
		}
		cols := strings.Split(line, "|")
		if len(cols) < 4 {
			continue
		}
		if title := strings.TrimSpace(cols[2]); title != "" {
			titles[normalizeTitle(title)] = true
		}
	}
	return titles, nil
}

// normalizeTitle reduces a title to a comparison key: lowercase, every run of
// non-alphanumeric characters collapsed to a single space, ends trimmed. Two titles
// that differ only in case, spacing or punctuation share a key.
func normalizeTitle(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevSpace = false
		} else if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// gitToplevel resolves the work-tree root containing dir (new-issue.sh prints paths
// relative to it).
func gitToplevel(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("resolving git toplevel of %s: %w: %s", dir, err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("resolving git toplevel of %s: %w", dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}
