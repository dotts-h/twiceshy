// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"strings"
	"testing"
)

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
