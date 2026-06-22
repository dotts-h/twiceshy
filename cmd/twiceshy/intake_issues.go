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
// entry is logged and removed so it cannot wedge a scheduled drain; an allocation or
// write failure aborts so the entry is retried next run. Issues land triage-flagged.
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
	issuesDir := filepath.Join(*repo, "docs", "issues")

	seen, err := existingIssueTitles(filepath.Join(issuesDir, "INDEX.md"))
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
		path, err := materializeIssue(*repo, newIssue, iss, today)
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

// materializeIssue allocates the next issue number via the canonical new-issue.sh
// (so numbering and the INDEX row never drift from the human path) and then rewrites
// the created file's body with the agent's description, category, author and any
// related record — re-screening the content so risky input is flagged before it
// lands in docs/issues. It returns the created file's absolute path.
func materializeIssue(repo, script string, iss spool.Issue, now string) (string, error) {
	// The title is passed as a single argv (no shell), so agent text cannot inject
	// flags or shell syntax; new-issue.sh takes $1 as the title verbatim.
	cmd := exec.Command("bash", script, iss.Title, "--severity", "medium")
	cmd.Dir = repo
	stdout, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("new-issue.sh: %w", err)
	}
	rel := strings.TrimSpace(string(stdout))
	if rel == "" {
		return "", errors.New("new-issue.sh printed no path")
	}
	// The script prints a path relative to the git toplevel it cd's to; resolve it
	// against that same toplevel (which -repo points into).
	root, err := gitToplevel(repo)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, rel)

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// Keep the script's frontmatter (the single source of the id/title/severity
	// shape) and replace the empty template body with the issue's content.
	front, _, ok := strings.Cut(string(content), "\n---\n")
	if !ok {
		return "", fmt.Errorf("created issue %s has no frontmatter delimiter", rel)
	}
	flags := screen.Flags(screen.Scan(iss.Title, iss.Description))
	if err := os.WriteFile(path, []byte(front+"\n---\n\n"+renderIssueBody(iss, now, flags)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// renderIssueBody fills a materialized issue with the agent's submission, mirroring
// the server's no-queue rendering (internal/server renderIssueMarkdown) so the two
// report_issue paths produce the same docs/issues shape. The title already lives in
// the script-written frontmatter; the screen flags ride the Notes.
func renderIssueBody(iss spool.Issue, now string, flags []string) string {
	var b strings.Builder
	b.WriteString("## Summary\n")
	fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(iss.Description))
	b.WriteString("## Notes\n")
	fmt.Fprintf(&b, "Agent-submitted via report_issue (category: %s) by %s", iss.Category, iss.Author)
	if iss.Session != "" {
		fmt.Fprintf(&b, " (session %s)", iss.Session)
	}
	fmt.Fprintf(&b, " on %s. Triage-flagged: never auto-actioned (#0066).", now)
	if iss.RelatedRecordID != "" {
		fmt.Fprintf(&b, " Related record: %s.", iss.RelatedRecordID)
	}
	if len(flags) > 0 {
		fmt.Fprintf(&b, " SECURITY flags: %s.", strings.Join(flags, ", "))
	}
	b.WriteString("\n")
	return b.String()
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
		return "", fmt.Errorf("resolving git toplevel of %s: %w", dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}
