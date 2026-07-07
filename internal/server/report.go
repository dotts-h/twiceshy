// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/spool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// reportDescription tells the model when to call report_outcome and that a
// report is a proposal, never a direct change to the served record.
const reportDescription = "Report that a served experience record's lesson did NOT work in practice, so the corpus can " +
	"correct itself. Call this after you followed a record (from search_experience/get_experience) and it " +
	"misfired: pass the record's `record_id`, a short `outcome` label (e.g. \"failed\", \"reproduced\", " +
	"\"incorrect\"), the failing repro or error in `evidence` (a reproducible artifact is the strongest " +
	"signal), and `author`. " +
	"This NEVER changes the record directly — it files a quarantined counter-record that the validation gate " +
	"adjudicates by re-running the original plus your counter. A bare report with no evidence is a triage flag, " +
	"not a demotion."

// maxOutcomeBytes bounds the outcome label (a short word, not prose).
const maxOutcomeBytes = 256

// ReportArgs is the input to the report_outcome tool.
type ReportArgs struct {
	RecordID string `json:"record_id" jsonschema:"id of the record whose lesson failed, e.g. exp-0042"`
	Outcome  string `json:"outcome" jsonschema:"short label of what happened, e.g. failed|reproduced|incorrect"`
	Evidence string `json:"evidence,omitempty" jsonschema:"the failing repro or error text; a reproducible artifact is the strongest signal"`
	Author   string `json:"author" jsonschema:"who is reporting"`
	Session  string `json:"session,omitempty"`
}

// ReportResult is the output of the report_outcome tool.
type ReportResult struct {
	RecordID string `json:"record_id"`          // the new quarantined counter-record id
	Disputes string `json:"disputes"`           // the disputed record id (unchanged)
	Markdown string `json:"markdown,omitempty"` // the quarantined counter-record to PR
	Message  string `json:"message"`
}

// reportOutcome processes a report_outcome tool call. It validates the disputed
// record exists, builds a quarantined counter-record (propose-only, like
// record_experience), and returns it. It NEVER mutates the disputed record and
// never writes to disk — "a report is evidence, not a verdict" (ADR-0013 §3).
func (h *handlers) reportOutcome(ctx context.Context, _ *mcp.CallToolRequest, args ReportArgs) (*mcp.CallToolResult, ReportResult, error) {
	start := time.Now()
	const tool = "report_outcome"

	tenant := TenantFromContext(ctx)
	alpha := isAlphaTenant(tenant)

	if err := validateReportSize(args); err != nil {
		h.logToolError(tool, start, err)
		return nil, ReportResult{}, err
	}
	if err := h.checkContributionQuota(ctx, tool, alphaContributionQuotas[tool]); err != nil {
		h.logToolError(tool, start, err)
		return nil, ReportResult{}, err
	}
	if strings.TrimSpace(args.Outcome) == "" {
		err := errors.New("outcome must be non-empty")
		h.logToolError(tool, start, err)
		return nil, ReportResult{}, err
	}
	if !record.ValidID(args.RecordID) {
		err := fmt.Errorf("record_id %q is not a valid record id (expected exp-NNNN)", args.RecordID)
		h.logToolError(tool, start, err)
		return nil, ReportResult{}, err
	}
	// The disputed record must exist — a report can only contest a real record.
	if _, err := h.ix.Get(ctx, args.RecordID); err != nil {
		h.logToolError(tool, start, err, slog.String("record_id", args.RecordID))
		return nil, ReportResult{}, fmt.Errorf("cannot report against %s: %w", args.RecordID, err)
	}

	// ADR-0031 full alpha posture (#0136): tighter size cap, then fail-closed
	// secret rejection — BEFORE any id allocation, spooling, or record
	// building, so a rejected submission never lands anywhere.
	if alpha {
		if len(args.Evidence) > alphaMaxEvidenceBytes {
			err := fmt.Errorf("evidence too large for an alpha tenant: %d bytes (max %d)", len(args.Evidence), alphaMaxEvidenceBytes)
			h.logToolError(tool, start, err)
			return nil, ReportResult{}, err
		}
		if err := rejectAlphaSecrets(tool, args.Evidence, args.Author, args.Session); err != nil {
			h.logToolError(tool, start, err)
			return nil, ReportResult{}, err
		}
	}

	id, err := h.allocateNextID(ctx)
	if err != nil {
		h.logToolError(tool, start, err)
		return nil, ReportResult{}, err
	}

	// TENANT ORIGIN STAMPING (#0128, ADR-0031): a tok_ tenant's counter-record
	// author is FORCED to "alpha:<token_id>" everywhere it is persisted —
	// same trust key and display-note pattern as record_experience. The
	// display note is prepended AFTER the secret scan above (it is
	// server-constructed, never scanned as untrusted input). Only prepended
	// when evidence is non-empty: ingest.BuildReport classifies a bare
	// report on strings.TrimSpace(Evidence)=="" (its own triage-flag
	// marking) — manufacturing non-empty evidence out of the note alone
	// would defeat that classification for exactly the hostile-tenant class
	// it exists to catch.
	reportAuthor, evidence := args.Author, args.Evidence
	if alpha {
		var display string
		reportAuthor, display = alphaStampAuthor(tenant, args.Author)
		if display != "" && strings.TrimSpace(evidence) != "" {
			evidence = fmt.Sprintf("_Submitted as: %s (untrusted alpha tenant; recorded origin: %s)_\n\n%s",
				display, reportAuthor, evidence)
		}
	}

	meta := ingest.Meta{ID: id, Author: reportAuthor, Now: time.Now().UTC().Format("2006-01-02")}
	if args.Session != "" {
		s := args.Session
		meta.Session = &s
	}

	rec, err := ingest.BuildReport(ingest.ReportInput{
		RecordID: args.RecordID,
		Outcome:  args.Outcome,
		Evidence: evidence,
	}, meta)
	if err != nil {
		h.logToolError(tool, start, err)
		return nil, ReportResult{}, err
	}

	md, err := record.Marshal(rec)
	if err != nil {
		h.logToolError(tool, start, err)
		return nil, ReportResult{}, err
	}

	// Automatic intake (ADR-0013 §E1): when a queue is configured, enqueue the
	// report so `intake-reports` materializes it into experience/ — no human
	// paste-PR. The queue stores the request, not this preview record: the final
	// id is allocated against the live corpus at intake (never colliding with
	// another report queued before the next drain). With no queue, the legacy
	// markdown-to-PR behavior is unchanged.
	queued := false
	if h.reportQueue != "" {
		if _, err := spool.Enqueue(h.reportQueue, spool.Report{
			RecordID:   args.RecordID,
			Outcome:    args.Outcome,
			Evidence:   evidence,
			Author:     reportAuthor,
			Session:    args.Session,
			ReportedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}); err != nil {
			h.logToolError(tool, start, err, slog.String("record_id", args.RecordID))
			return nil, ReportResult{}, fmt.Errorf("queueing report against %s for intake: %w", args.RecordID, err)
		}
		queued = true
	}

	var msg string
	if queued {
		msg = fmt.Sprintf("Outcome report against %s queued for automatic intake — no PR needed. The nightly intake materializes it "+
			"as a quarantined counter-record (a fresh id is assigned then) that the validation gate (#0032) adjudicates; it does NOT change %s.", args.RecordID, args.RecordID)
	} else {
		msg = fmt.Sprintf("Quarantined counter-record %s created disputing %s — open it as a PR; it does NOT change %s. "+
			"The validation gate (#0032) will turn it into a repro and adjudicate.", rec.ID, args.RecordID, args.RecordID)
	}
	if flags := rec.Provenance.SecurityFlags; len(flags) > 0 {
		msg += " SECURITY: the safety gate flagged the evidence (" + strings.Join(flags, ", ") + ")."
	}

	result := ReportResult{
		RecordID: rec.ID,
		Disputes: args.RecordID,
		Markdown: string(md),
		Message:  msg,
	}
	h.logger.Info("tool call",
		slog.String("tool", tool),
		slog.String("outcome", "ok"),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.String("record_id", rec.ID),
		slog.String("disputes", args.RecordID),
	)
	return nil, result, nil
}

// validateReportSize rejects oversized inputs cheaply, before NextID and the
// existence probe. Evidence is body-like (a failing repro/error), so it reuses
// the record body cap; the outcome is a short label.
func validateReportSize(args ReportArgs) error {
	if len(args.Evidence) > maxRecordBodyBytes {
		return fmt.Errorf("evidence too large: %d bytes (max %d)", len(args.Evidence), maxRecordBodyBytes)
	}
	if len(args.Outcome) > maxOutcomeBytes {
		return fmt.Errorf("outcome too large: %d bytes (max %d)", len(args.Outcome), maxOutcomeBytes)
	}
	return nil
}
