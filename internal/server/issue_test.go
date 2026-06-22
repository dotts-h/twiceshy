// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/spool"
)

// With a queue configured, report_issue enqueues the half-formed input for
// `intake-issues` (it is never silently lost) and reports that it was queued.
func TestReportIssue_QueuesWhenConfigured(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	queue := t.TempDir()
	h.issueQueue = queue

	_, res, err := h.reportIssue(context.Background(), nil, IssueArgs{
		Title:       "search_experience returns 500 on a query with a NUL byte",
		Description: "I sent a query containing a literal NUL and got a 500; no record, no fix yet.",
		Category:    "bug", Author: "agent-x",
	})
	if err != nil {
		t.Fatalf("reportIssue: %v", err)
	}
	if !strings.Contains(res.Message, "queued") {
		t.Fatalf("a queued issue must say so: %q", res.Message)
	}
	paths, err := spool.List(queue)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("want exactly 1 queued issue, got %d", len(paths))
	}
	iss, err := spool.ReadIssue(paths[0])
	if err != nil {
		t.Fatalf("ReadIssue: %v", err)
	}
	if iss.Category != "bug" || !strings.Contains(iss.Title, "NUL byte") || iss.Author != "agent-x" {
		t.Fatalf("queued issue lost data: %+v", iss)
	}
}

// With no queue, report_issue still returns a PR-ready docs/issues markdown so the
// submission is captured (the half-formed surface the agent otherwise lacked).
func TestReportIssue_NoQueueReturnsMarkdown(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	_, res, err := h.reportIssue(context.Background(), nil, IssueArgs{
		Title: "add a -since flag to the osv importer", Description: "would like to bound the import window.",
		Category: "feature", Author: "agent-y",
	})
	if err != nil {
		t.Fatalf("reportIssue: %v", err)
	}
	if strings.Contains(res.Message, "queued") {
		t.Fatalf("no queue must not claim queued: %q", res.Message)
	}
	for _, want := range []string{"## Summary", "title:", "status: open", "add a -since flag"} {
		if !strings.Contains(res.Markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, res.Markdown)
		}
	}
}

func TestReportIssue_RejectsBadCategory(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	if _, _, err := h.reportIssue(context.Background(), nil, IssueArgs{
		Title: "x is broken", Description: "d", Category: "nonsense", Author: "a",
	}); err == nil {
		t.Fatal("an unknown category must be rejected")
	}
}

func TestReportIssue_RequiresTitle(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	if _, _, err := h.reportIssue(context.Background(), nil, IssueArgs{
		Title: "   ", Description: "d", Category: "bug", Author: "a",
	}); err == nil {
		t.Fatal("an empty title must be rejected")
	}
}

func TestReportIssue_RejectsOversizedDescription(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	if _, _, err := h.reportIssue(context.Background(), nil, IssueArgs{
		Title: "t", Description: strings.Repeat("x", maxRecordBodyBytes+1), Category: "bug", Author: "a",
	}); err == nil {
		t.Fatal("an oversized description must be rejected")
	}
}

func TestReportIssue_RejectsMalformedRelatedID(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	if _, _, err := h.reportIssue(context.Background(), nil, IssueArgs{
		Title: "t", Description: "d", Category: "question", Author: "a", RelatedRecordID: "not-an-id",
	}); err == nil {
		t.Fatal("a malformed related_record_id must be rejected")
	}
}

// The content screen is inherited (secrets/PII never reach docs/issues silently).
func TestReportIssue_ScreensSecrets(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	_, res, err := h.reportIssue(context.Background(), nil, IssueArgs{
		Title: "crash with creds in the log", Description: "the log printed AKIAIOSFODNN7EXAMPLE before crashing.",
		Category: "bug", Author: "a",
	})
	if err != nil {
		t.Fatalf("reportIssue: %v", err)
	}
	if !strings.Contains(strings.ToLower(res.Message), "security") {
		t.Fatalf("a secret in the description must raise a security flag: %q", res.Message)
	}
}
