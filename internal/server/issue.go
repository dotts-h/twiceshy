// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/screen"
	"github.com/dotts-h/twiceshy/internal/spool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// issueDescription tells the model when to call report_issue — the surface for
// HALF-FORMED input that record_experience (needs a complete lesson) and
// report_outcome (needs an existing record) reject, so it is no longer lost.
const issueDescription = "File a half-formed issue when you have NO complete lesson to record yet: a problem you hit but " +
	"haven't solved, a missing feature, an open question, or a bug in twiceshy itself (it returned garbage / crashed). " +
	"Pass a short `title`, a `description` of what happened or what you want, a `category` (bug|feature|question), " +
	"`author`, and optionally `related_record_id` if a served record is involved. This is NOT record_experience " +
	"(which needs root_cause+fix+guarding_test) and NOT report_outcome (which disputes an existing record) — it is the " +
	"intake for input those reject. The issue is triage-flagged and NEVER auto-actioned: a human or the triage doctor " +
	"promotes it."

// maxIssueTitleBytes bounds the title (a one-line summary, not prose). The
// description reuses the record-body cap.
const maxIssueTitleBytes = 256

// issueCategories is the closed set the contract allows.
var issueCategories = map[string]bool{"bug": true, "feature": true, "question": true}

// IssueArgs is the input to the report_issue tool.
type IssueArgs struct {
	Title           string `json:"title" jsonschema:"one-line summary of the problem, request, or question"`
	Description     string `json:"description" jsonschema:"what happened or what you want; no fix is required"`
	Category        string `json:"category" jsonschema:"one of bug|feature|question"`
	RelatedRecordID string `json:"related_record_id,omitempty" jsonschema:"id of a served record this concerns, e.g. exp-0042 (optional)"`
	Author          string `json:"author" jsonschema:"who is filing"`
	Session         string `json:"session,omitempty"`
}

// IssueResult is the output of the report_issue tool.
type IssueResult struct {
	Category string `json:"category"`
	Markdown string `json:"markdown,omitempty"` // a PR-ready docs/issues entry (no id; allocated at intake)
	Message  string `json:"message"`
}

// reportIssue captures half-formed agent input. When an issue queue is configured
// it enqueues the request for `intake-issues` (materialized into docs/issues/ with
// a fresh number); otherwise it returns a PR-ready docs/issues markdown so the
// submission is never silently lost. It writes no docs/issues file directly and
// never auto-actions — an agent-submitted issue is triage-flagged (#0066).
func (h *handlers) reportIssue(_ context.Context, _ *mcp.CallToolRequest, args IssueArgs) (*mcp.CallToolResult, IssueResult, error) {
	start := time.Now()
	const tool = "report_issue"

	if err := validateIssueSize(args); err != nil {
		h.logToolError(tool, start, err)
		return nil, IssueResult{}, err
	}
	title := strings.TrimSpace(args.Title)
	if title == "" {
		err := errors.New("title must be non-empty")
		h.logToolError(tool, start, err)
		return nil, IssueResult{}, err
	}
	cat := strings.ToLower(strings.TrimSpace(args.Category))
	if !issueCategories[cat] {
		err := fmt.Errorf("category %q must be one of bug, feature, question", args.Category)
		h.logToolError(tool, start, err)
		return nil, IssueResult{}, err
	}
	if args.RelatedRecordID != "" && !record.ValidID(args.RelatedRecordID) {
		err := fmt.Errorf("related_record_id %q is not a valid record id (expected exp-NNNN)", args.RelatedRecordID)
		h.logToolError(tool, start, err)
		return nil, IssueResult{}, err
	}

	// Content screen (secrets/PII/harmful) — inherited like record_experience. An
	// agent-submitted issue is mirrored to docs/issues + Forgejo, so flag risky
	// content before it lands; the flags ride the message and the rendered Notes.
	flags := screen.Flags(screen.Scan(title, args.Description))

	md := renderIssueMarkdown(title, args.Description, cat, args.RelatedRecordID, args.Author, args.Session, time.Now().UTC(), flags)

	queued := false
	if h.issueQueue != "" {
		if _, err := spool.EnqueueIssue(h.issueQueue, spool.Issue{
			Title:           title,
			Description:     args.Description,
			Category:        cat,
			RelatedRecordID: args.RelatedRecordID,
			Author:          args.Author,
			Session:         args.Session,
			ReportedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		}); err != nil {
			h.logToolError(tool, start, err)
			return nil, IssueResult{}, fmt.Errorf("queueing issue for intake: %w", err)
		}
		queued = true
	}

	var msg string
	if queued {
		msg = fmt.Sprintf("Issue %q queued for triage intake — captured, never silently lost. `intake-issues` materializes it into docs/issues/ "+
			"(a number is assigned then); it is triage-flagged and never auto-actioned (#0066).", title)
	} else {
		msg = fmt.Sprintf("Issue %q rendered as a PR-ready docs/issues entry below (no intake queue configured) — open it as a PR. "+
			"It is triage-flagged, not auto-actioned.", title)
	}
	if len(flags) > 0 {
		msg += " SECURITY: the safety gate flagged the content (" + strings.Join(flags, ", ") + ")."
	}

	h.logger.Info("tool call",
		slog.String("tool", tool),
		slog.String("outcome", "ok"),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.String("category", cat),
		slog.Bool("queued", queued),
	)
	return nil, IssueResult{Category: cat, Markdown: md, Message: msg}, nil
}

// validateIssueSize rejects oversized inputs cheaply (the global body cap also
// bounds the whole request; this gives a precise per-field error).
func validateIssueSize(args IssueArgs) error {
	if len(args.Title) > maxIssueTitleBytes {
		return fmt.Errorf("title too large: %d bytes (max %d)", len(args.Title), maxIssueTitleBytes)
	}
	if len(args.Description) > maxRecordBodyBytes {
		return fmt.Errorf("description too large: %d bytes (max %d)", len(args.Description), maxRecordBodyBytes)
	}
	return nil
}

// renderIssueMarkdown renders a docs/issues entry (TEMPLATE.md shape) with no id —
// the number is allocated at intake. The title is %q-quoted so agent-controlled
// text cannot inject YAML into the frontmatter.
func renderIssueMarkdown(title, description, category, relatedID, author, session string, now time.Time, flags []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id:\n") // allocated at intake (intake-issues / new-issue.sh)
	fmt.Fprintf(&b, "title: %q\n", title)
	b.WriteString("status: open\n")
	b.WriteString("severity: medium\n")
	b.WriteString("group:\n")
	b.WriteString("depends_on: []\n")
	b.WriteString("forgejo:\n")
	b.WriteString("links:\n  adr:\n  prs: []\n  issues: []\n  regression:\n")
	b.WriteString("assets: []\n")
	b.WriteString("---\n\n")
	b.WriteString("## Summary\n")
	fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(description))
	b.WriteString("## Notes\n")
	fmt.Fprintf(&b, "Agent-submitted via report_issue (category: %s) by %s", category, author)
	if session != "" {
		fmt.Fprintf(&b, " (session %s)", session)
	}
	fmt.Fprintf(&b, " on %s. Triage-flagged: never auto-actioned (#0066).", now.Format("2006-01-02"))
	if relatedID != "" {
		fmt.Fprintf(&b, " Related record: %s.", relatedID)
	}
	if len(flags) > 0 {
		fmt.Fprintf(&b, " SECURITY flags: %s.", strings.Join(flags, ", "))
	}
	b.WriteString("\n")
	return b.String()
}
