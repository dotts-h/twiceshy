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

// TestReportIssue_AlphaTenantStampsOriginAndPreservesDisplayNote is issue
// 0136's repro applied to report_issue (ADR-0031): a spoofed importer author
// is stamped to alpha:<tenant> in the rendered markdown and the spooled
// Issue.Author, everywhere it is persisted; the caller string survives only
// as a display note in the description.
func TestReportIssue_AlphaTenantStampsOriginAndPreservesDisplayNote(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.contribQuota = &fakeContributionQuota{}
	queue := t.TempDir()
	h.issueQueue = queue
	ctx := withTenant(context.Background(), "tok_alpha0001")

	_, res, err := h.reportIssue(ctx, nil, IssueArgs{
		Title: "an alpha-submitted issue", Description: "the importer path is broken",
		Category: "bug", Author: "osv-importer",
	})
	if err != nil {
		t.Fatalf("reportIssue: %v", err)
	}
	if !strings.Contains(res.Markdown, "alpha:tok_alpha0001") {
		t.Errorf("rendered markdown must carry the stamped alpha:tok_alpha0001 origin:\n%s", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "Submitted as: osv-importer (untrusted alpha tenant; recorded origin: alpha:tok_alpha0001)") {
		t.Errorf("caller author must survive as a display note in the description:\n%s", res.Markdown)
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
	if iss.Author != "alpha:tok_alpha0001" {
		t.Errorf("spooled Issue.Author = %q, want alpha:tok_alpha0001", iss.Author)
	}
	if !strings.Contains(iss.Description, "Submitted as: osv-importer") {
		t.Errorf("spooled Issue.Description must carry the display note: %q", iss.Description)
	}
}

// TestReportIssue_AlphaBareDescriptionStaysBare is report_issue's mirror of
// TestReportOutcome_AlphaBareReportStaysBare: an alpha tenant with a
// non-empty author but an EMPTY description must stay bare — no "Submitted
// as" note manufactured into the description (empty descriptions are legal;
// validateIssueSize only caps size, never requires non-empty).
func TestReportIssue_AlphaBareDescriptionStaysBare(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.contribQuota = &fakeContributionQuota{}
	queue := t.TempDir()
	h.issueQueue = queue
	ctx := withTenant(context.Background(), "tok_alpha0001")

	_, res, err := h.reportIssue(ctx, nil, IssueArgs{
		Title: "an alpha-submitted issue with no description", Description: "",
		Category: "bug", Author: "someone",
	})
	if err != nil {
		t.Fatalf("reportIssue: %v", err)
	}
	if strings.Contains(res.Markdown, "Submitted as") {
		t.Errorf("an empty-description alpha issue must NOT manufacture a display note:\n%s", res.Markdown)
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
	if iss.Description != "" {
		t.Errorf("spooled Issue.Description = %q, want empty (bare description stays bare)", iss.Description)
	}
}

// TestReportIssue_AlphaTenantSessionSecretRejected guards a gap the
// second-opinion review found: `session` is caller-supplied free text that
// gets persisted (the spooled Issue.Session) but was not covered by the
// alpha fail-closed secret scan. A clean title/description but a
// secret-shaped session must still be rejected outright for an alpha
// tenant — nothing queued.
func TestReportIssue_AlphaTenantSessionSecretRejected(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.contribQuota = &fakeContributionQuota{}
	queue := t.TempDir()
	h.issueQueue = queue
	ctx := withTenant(context.Background(), "tok_alpha0001")

	secret := "ghp_" + strings.Repeat("A", 36)
	_, _, err := h.reportIssue(ctx, nil, IssueArgs{
		Title: "a perfectly clean title", Description: "a perfectly clean description",
		Category: "bug", Author: "a", Session: secret,
	})
	if err == nil {
		t.Fatal("an alpha tenant's secret-shaped session must be a tool error, not stored")
	}
	if !strings.Contains(err.Error(), "secret:github-token") {
		t.Errorf("error = %q, want it to name the secret:github-token rule", err.Error())
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error = %q, must never echo the raw secret", err.Error())
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Errorf("a rejected alpha issue (session secret) queued %d files, want 0", len(paths))
	}
}

// TestReportIssue_OperatorSessionSecretUnaffected is the regression half:
// the operator channel's behavior around `session` is unchanged by this fix
// (the alpha fail-closed scan is alpha-only).
func TestReportIssue_OperatorSessionSecretUnaffected(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	ctx := withTenant(context.Background(), "operator")

	secret := "ghp_" + strings.Repeat("A", 36)
	if _, _, err := h.reportIssue(ctx, nil, IssueArgs{
		Title: "a perfectly clean title", Description: "a perfectly clean description",
		Category: "bug", Author: "a", Session: secret,
	}); err != nil {
		t.Fatalf("operator behavior around session must be unchanged: %v", err)
	}
}

// TestReportIssue_OperatorAuthorUnchanged is the regression half: the
// operator tenant's author is exactly the caller-supplied value, with no
// display note.
func TestReportIssue_OperatorAuthorUnchanged(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	ctx := withTenant(context.Background(), "operator")

	_, res, err := h.reportIssue(ctx, nil, IssueArgs{
		Title: "an operator-submitted issue", Description: "d",
		Category: "bug", Author: "claude-operator",
	})
	if err != nil {
		t.Fatalf("reportIssue: %v", err)
	}
	if !strings.Contains(res.Markdown, "claude-operator") {
		t.Errorf("operator author must be unchanged:\n%s", res.Markdown)
	}
	if strings.Contains(res.Markdown, "Submitted as:") {
		t.Errorf("an operator issue must not carry the alpha display note:\n%s", res.Markdown)
	}
}

// TestReportIssue_AlphaTenantSecretRejectedNothingQueued is ADR-0031's
// fail-closed secret posture: secret-shaped content from an alpha tenant is
// REJECTED outright — nothing queued, nothing rendered as stored.
func TestReportIssue_AlphaTenantSecretRejectedNothingQueued(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.contribQuota = &fakeContributionQuota{}
	queue := t.TempDir()
	h.issueQueue = queue
	ctx := withTenant(context.Background(), "tok_alpha0001")

	secret := "AKIA" + strings.Repeat("A", 16)
	_, _, err := h.reportIssue(ctx, nil, IssueArgs{
		Title: "crash with creds", Description: "the log printed " + secret,
		Category: "bug", Author: "a",
	})
	if err == nil {
		t.Fatal("an alpha tenant's secret-shaped description must be a tool error, not stored")
	}
	if !strings.Contains(err.Error(), "secret:aws-access-key") {
		t.Errorf("error = %q, want it to name the secret:aws-access-key rule", err.Error())
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error = %q, must never echo the raw secret", err.Error())
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Errorf("a rejected alpha issue queued %d files, want 0", len(paths))
	}
}

// TestReportIssue_AlphaTenantDescriptionOverCapRejected is ADR-0031's alpha
// size-cap invariant: a description over alphaMaxIssueDescriptionBytes is
// rejected for an alpha tenant, at the exact boundary.
func TestReportIssue_AlphaTenantDescriptionOverCapRejected(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.contribQuota = &fakeContributionQuota{}
	ctx := withTenant(context.Background(), "tok_alpha0001")

	_, _, err := h.reportIssue(ctx, nil, IssueArgs{
		Title: "t", Description: strings.Repeat("d", alphaMaxIssueDescriptionBytes), Category: "bug", Author: "a",
	})
	if err != nil {
		t.Fatalf("description at the alpha cap must be accepted: %v", err)
	}

	_, _, err = h.reportIssue(ctx, nil, IssueArgs{
		Title: "t", Description: strings.Repeat("d", alphaMaxIssueDescriptionBytes+1), Category: "bug", Author: "a",
	})
	if err == nil {
		t.Fatal("a description one byte over the alpha cap must be rejected")
	}
	if !strings.Contains(err.Error(), "description too large for an alpha tenant") {
		t.Errorf("error = %q, want the alpha-tenant description-cap message", err.Error())
	}
}
