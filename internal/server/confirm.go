// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// confirmDescription tells the model when to call confirm_helpful and that it
// records a reinforcement signal, never a direct change to the served record.
const confirmDescription = "Record that a served experience record's lesson actually worked in practice. " +
	"Call this after you followed a record (from search_experience/get_experience) and it helped: pass the " +
	"record's `record_id` and optionally `author`. " +
	"This records a reinforcement signal (confirmed_helpful); it does NOT change the record text. " +
	"It is the positive counterpart to report_outcome."

// ConfirmArgs is the input to the confirm_helpful tool.
type ConfirmArgs struct {
	RecordID string `json:"record_id" jsonschema:"id of the record whose lesson worked, e.g. exp-0042"`
	Author   string `json:"author,omitempty" jsonschema:"who is confirming"`
	Session  string `json:"session,omitempty"`
}

// ConfirmResult is the output of the confirm_helpful tool.
type ConfirmResult struct {
	RecordID string `json:"record_id"`
	Message  string `json:"message"`
}

// confirmHelpful processes a confirm_helpful tool call. It validates the record
// exists and increments its confirmed_helpful usage counter — a direct
// best-effort signal, not a propose-only path.
func (h *handlers) confirmHelpful(ctx context.Context, _ *mcp.CallToolRequest, args ConfirmArgs) (*mcp.CallToolResult, ConfirmResult, error) {
	start := time.Now()
	const tool = "confirm_helpful"

	if !record.ValidID(args.RecordID) {
		err := fmt.Errorf("record_id %q is not a valid record id (expected exp-NNNN)", args.RecordID)
		h.logToolError(tool, start, err)
		return nil, ConfirmResult{}, err
	}
	if _, err := h.ix.Get(ctx, args.RecordID); err != nil {
		h.logToolError(tool, start, err, slog.String("record_id", args.RecordID))
		return nil, ConfirmResult{}, fmt.Errorf("cannot confirm helpful for %s: %w", args.RecordID, err)
	}
	if err := h.ix.ConfirmHelpful(ctx, args.RecordID); err != nil {
		h.logToolError(tool, start, err, slog.String("record_id", args.RecordID))
		return nil, ConfirmResult{}, err
	}

	msg := fmt.Sprintf("Recorded a positive signal for %s (confirmed_helpful); this reinforces the record, it does not change its text.", args.RecordID)
	h.logger.Info("tool call",
		slog.String("tool", tool),
		slog.String("outcome", "ok"),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.String("record_id", args.RecordID),
	)
	return nil, ConfirmResult{RecordID: args.RecordID, Message: msg}, nil
}
