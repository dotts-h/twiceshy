// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/spool"
)

// With a queue configured, report_outcome enqueues the report for automatic
// intake (no human paste-PR) instead of just returning markdown (#0042).
func TestReportOutcome_QueuesForIntakeWhenConfigured(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture()) // corpus holds exp-0200
	queue := t.TempDir()
	h.reportQueue = queue

	_, res, err := h.reportOutcome(context.Background(), nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Evidence: "still errors", Author: "agent-x",
	})
	if err != nil {
		t.Fatalf("reportOutcome: %v", err)
	}
	if !strings.Contains(res.Message, "queued") {
		t.Fatalf("a queued report must say so in the message: %q", res.Message)
	}

	paths, err := spool.List(queue)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("want exactly 1 queued report, got %d", len(paths))
	}
	rep, err := spool.Read(paths[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if rep.RecordID != "exp-0200" || rep.Outcome != "failed" || rep.Evidence != "still errors" {
		t.Fatalf("queued report lost data: %+v", rep)
	}
}

// With no queue configured the legacy behavior holds: markdown returned, nothing
// claimed queued.
func TestReportOutcome_NoQueueReturnsMarkdown(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	_, res, err := h.reportOutcome(context.Background(), nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Author: "agent-x",
	})
	if err != nil {
		t.Fatalf("reportOutcome: %v", err)
	}
	if res.Markdown == "" {
		t.Fatal("the legacy path must return the counter-record markdown to PR")
	}
	if strings.Contains(res.Message, "queued") {
		t.Fatalf("the no-queue path must not claim the report was queued: %q", res.Message)
	}
}

func TestReportOutcome_BuildsQuarantinedCounterRecord(t *testing.T) {
	h, ix := newUsageHandlers(t, usageFixture()) // corpus holds exp-0200 (validated)

	_, res, err := h.reportOutcome(context.Background(), nil, ReportArgs{
		RecordID: "exp-0200",
		Outcome:  "failed",
		Evidence: "go build ./... still errors on io/ioutil after applying the fix",
		Author:   "agent-x",
	})
	if err != nil {
		t.Fatalf("reportOutcome: %v", err)
	}
	if res.Disputes != "exp-0200" {
		t.Errorf("disputes = %q, want exp-0200", res.Disputes)
	}
	if res.RecordID == "" || res.RecordID == "exp-0200" {
		t.Errorf("counter-record id = %q, want a fresh allocated id, not the disputed one", res.RecordID)
	}
	if !strings.Contains(res.Markdown, "disputes: exp-0200") {
		t.Errorf("counter-record markdown missing the disputes link:\n%s", res.Markdown)
	}

	// Propose-only: the disputed record must be untouched (still validated).
	stored, err := ix.Get(context.Background(), "exp-0200")
	if err != nil {
		t.Fatalf("disputed record vanished: %v", err)
	}
	if stored.Status != "validated" {
		t.Errorf("report mutated the disputed record's status to %q (must be propose-only)", stored.Status)
	}
}

// Two report_outcome previews in one session (no queue) must allocate DISTINCT
// counter-record ids. ingest.NextID is corpus-derived and stateless — the same
// collision class record_experience fixed via the serialized allocator (#0089).
// Without routing report_outcome through h.allocateNextID, two concurrent reports
// (or a report racing a record_experience) hand back paste-PR markdown carrying
// the same exp-NNNN.
func TestReportOutcome_AllocatesDistinctIDsInOneSession(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture()) // corpus holds exp-0200, no queue
	id := func() string {
		t.Helper()
		_, res, err := h.reportOutcome(context.Background(), nil, ReportArgs{
			RecordID: "exp-0200", Outcome: "failed", Author: "agent-x",
		})
		if err != nil {
			t.Fatalf("reportOutcome: %v", err)
		}
		return res.RecordID
	}
	id1, id2 := id(), id()
	if id1 == id2 {
		t.Errorf("two report_outcome previews collided on id %q — must be distinct (#0089 class)", id1)
	}
}

func TestReportOutcome_RejectsUnknownRecord(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	if _, _, err := h.reportOutcome(context.Background(), nil, ReportArgs{
		RecordID: "exp-9999", Outcome: "failed", Author: "agent-x",
	}); err == nil {
		t.Fatal("a report against a non-existent record must be rejected")
	}
}

func TestReportOutcome_RejectsMalformedRecordID(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	if _, _, err := h.reportOutcome(context.Background(), nil, ReportArgs{
		RecordID: "not-an-id", Outcome: "failed", Author: "agent-x",
	}); err == nil {
		t.Fatal("a malformed record_id must be rejected before any work")
	}
}

func TestReportOutcome_RejectsOversizedEvidence(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	if _, _, err := h.reportOutcome(context.Background(), nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Author: "agent-x",
		Evidence: strings.Repeat("x", maxRecordBodyBytes+1),
	}); err == nil {
		t.Fatal("oversized evidence must be rejected")
	}
}

func TestReportOutcome_RequiresOutcome(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	if _, _, err := h.reportOutcome(context.Background(), nil, ReportArgs{
		RecordID: "exp-0200", Author: "agent-x",
	}); err == nil {
		t.Fatal("an empty outcome must be rejected")
	}
}

// TestReportOutcome_AlphaTenantStampsOriginAndPreservesDisplayNote is issue
// 0136's repro (ADR-0031): an alpha tenant calling report_outcome with a
// spoofed importer author gets that author stamped to alpha:<tenant> in
// provenance.source.author and the spooled Report.Author, everywhere it is
// persisted — the caller string survives only as a display note in the
// evidence text.
func TestReportOutcome_AlphaTenantStampsOriginAndPreservesDisplayNote(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.contribQuota = &fakeContributionQuota{}
	queue := t.TempDir()
	h.reportQueue = queue
	ctx := withTenant(context.Background(), "tok_alpha0001")

	_, res, err := h.reportOutcome(ctx, nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Evidence: "still errors", Author: "osv-importer",
	})
	if err != nil {
		t.Fatalf("reportOutcome: %v", err)
	}
	if !strings.Contains(res.Markdown, "author: alpha:tok_alpha0001") {
		t.Errorf("counter-record markdown must carry provenance.source.author = alpha:tok_alpha0001:\n%s", res.Markdown)
	}
	if strings.Contains(res.Markdown, "author: osv-importer") {
		t.Fatal("the caller-supplied author must never become provenance.source.author (spoofed origin)")
	}
	if !strings.Contains(res.Markdown, "Submitted as: osv-importer (untrusted alpha tenant; recorded origin: alpha:tok_alpha0001)") {
		t.Errorf("caller author must survive as a display note in the evidence:\n%s", res.Markdown)
	}

	paths, err := spool.List(queue)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("want exactly 1 queued report, got %d", len(paths))
	}
	rep, err := spool.Read(paths[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if rep.Author != "alpha:tok_alpha0001" {
		t.Errorf("spooled Report.Author = %q, want alpha:tok_alpha0001", rep.Author)
	}
	if !strings.Contains(rep.Evidence, "Submitted as: osv-importer") {
		t.Errorf("spooled Report.Evidence must carry the display note: %q", rep.Evidence)
	}
}

// TestReportOutcome_AlphaBareReportStaysBare guards against the display note
// defeating ingest.BuildReport's bare-report triage classification
// (internal/ingest/report.go:51-70 keys off strings.TrimSpace(Evidence)==""):
// an alpha tenant with a non-empty author but EMPTY evidence must stay bare
// — no "Submitted as" note manufactured into the evidence, so a
// zero-evidence hostile-tenant report still reads as a bare triage flag,
// never as evidenced.
func TestReportOutcome_AlphaBareReportStaysBare(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.contribQuota = &fakeContributionQuota{}
	queue := t.TempDir()
	h.reportQueue = queue
	ctx := withTenant(context.Background(), "tok_alpha0001")

	_, res, err := h.reportOutcome(ctx, nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Evidence: "", Author: "someone",
	})
	if err != nil {
		t.Fatalf("reportOutcome: %v", err)
	}
	if !strings.Contains(res.Markdown, "No reproducible artifact provided") {
		t.Errorf("an empty-evidence alpha report must keep the bare-report triage marking:\n%s", res.Markdown)
	}
	if strings.Contains(res.Markdown, "Submitted as") {
		t.Errorf("an empty-evidence alpha report must NOT manufacture a display note (defeats bare-report triage):\n%s", res.Markdown)
	}

	paths, err := spool.List(queue)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("want exactly 1 queued report, got %d", len(paths))
	}
	rep, err := spool.Read(paths[0])
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if rep.Evidence != "" {
		t.Errorf("spooled Report.Evidence = %q, want empty (bare report stays bare)", rep.Evidence)
	}
}

// TestReportOutcome_AlphaTenantSessionSecretRejected guards a gap the
// second-opinion review found: `session` is caller-supplied free text that
// gets persisted (ingest.Meta.Session → provenance.session, and the spooled
// Report.Session) but was not covered by the alpha fail-closed secret scan.
// A clean evidence field but a secret-shaped session must still be rejected
// outright for an alpha tenant — nothing spooled.
func TestReportOutcome_AlphaTenantSessionSecretRejected(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.contribQuota = &fakeContributionQuota{}
	queue := t.TempDir()
	h.reportQueue = queue
	ctx := withTenant(context.Background(), "tok_alpha0001")

	secret := "ghp_" + strings.Repeat("A", 36)
	_, _, err := h.reportOutcome(ctx, nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Evidence: "a perfectly clean evidence text", Author: "a", Session: secret,
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
		t.Errorf("a rejected alpha report (session secret) spooled %d files, want 0", len(paths))
	}
}

// TestReportOutcome_OperatorSessionSecretUnaffected is the regression half:
// the operator channel's behavior around `session` is unchanged by this fix
// (the alpha fail-closed scan is alpha-only).
func TestReportOutcome_OperatorSessionSecretUnaffected(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	ctx := withTenant(context.Background(), "operator")

	secret := "ghp_" + strings.Repeat("A", 36)
	if _, _, err := h.reportOutcome(ctx, nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Evidence: "a perfectly clean evidence text", Author: "a", Session: secret,
	}); err != nil {
		t.Fatalf("operator behavior around session must be unchanged: %v", err)
	}
}

// TestReportOutcome_OperatorAuthorUnchanged is the regression half: the
// operator tenant's author is exactly the caller-supplied value, byte for
// byte, with no display note.
func TestReportOutcome_OperatorAuthorUnchanged(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	ctx := withTenant(context.Background(), "operator")

	_, res, err := h.reportOutcome(ctx, nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Evidence: "still errors", Author: "claude-operator",
	})
	if err != nil {
		t.Fatalf("reportOutcome: %v", err)
	}
	if !strings.Contains(res.Markdown, "author: claude-operator") {
		t.Errorf("operator author must be unchanged:\n%s", res.Markdown)
	}
	if strings.Contains(res.Markdown, "Submitted as:") {
		t.Errorf("an operator report must not carry the alpha display note:\n%s", res.Markdown)
	}
}

// TestReportOutcome_AlphaTenantSecretRejectedNothingSpooled is ADR-0031's
// fail-closed secret posture: secret-shaped evidence from an alpha tenant is
// REJECTED outright — nothing is spooled, nothing is stored.
func TestReportOutcome_AlphaTenantSecretRejectedNothingSpooled(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.contribQuota = &fakeContributionQuota{}
	queue := t.TempDir()
	h.reportQueue = queue
	ctx := withTenant(context.Background(), "tok_alpha0001")

	secret := "AKIA" + strings.Repeat("A", 16)
	_, _, err := h.reportOutcome(ctx, nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Evidence: "leaked: " + secret, Author: "agent-x",
	})
	if err == nil {
		t.Fatal("an alpha tenant's secret-shaped evidence must be a tool error, not stored")
	}
	if !strings.Contains(err.Error(), "secret:aws-access-key") {
		t.Errorf("error = %q, want it to name the secret:aws-access-key rule", err.Error())
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error = %q, must never echo the raw secret", err.Error())
	}
	if paths, _ := spool.List(queue); len(paths) != 0 {
		t.Errorf("a rejected alpha report spooled %d files, want 0", len(paths))
	}
}

// TestReportOutcome_AlphaTenantEvidenceOverCapRejected is ADR-0031's alpha
// size-cap invariant: evidence over alphaMaxEvidenceBytes is rejected for an
// alpha tenant, at the exact boundary.
func TestReportOutcome_AlphaTenantEvidenceOverCapRejected(t *testing.T) {
	h, _ := newUsageHandlers(t, usageFixture())
	h.contribQuota = &fakeContributionQuota{}
	ctx := withTenant(context.Background(), "tok_alpha0001")

	_, _, err := h.reportOutcome(ctx, nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Evidence: strings.Repeat("e", alphaMaxEvidenceBytes), Author: "a",
	})
	if err != nil {
		t.Fatalf("evidence at the alpha cap must be accepted: %v", err)
	}

	_, _, err = h.reportOutcome(ctx, nil, ReportArgs{
		RecordID: "exp-0200", Outcome: "failed", Evidence: strings.Repeat("e", alphaMaxEvidenceBytes+1), Author: "a",
	})
	if err == nil {
		t.Fatal("evidence one byte over the alpha cap must be rejected")
	}
	if !strings.Contains(err.Error(), "evidence too large for an alpha tenant") {
		t.Errorf("error = %q, want the alpha-tenant evidence-cap message", err.Error())
	}
}
