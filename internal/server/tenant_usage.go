// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tenantCallRecorder is the slice of the index the tool-call telemetry path
// needs (#0126): an atomic per-tenant, per-tool call counter. Narrowed to an
// interface, like usageStore/TokenStore, so it is unit-testable with a stub
// and documents that this write can never influence a tool call's outcome.
type tenantCallRecorder interface {
	CountTenantCall(tokenID, tool string, now time.Time) error
}

// recordTenantCall is the hard-rule-guarded write for per-tenant telemetry
// (#0126): a failing or missing recorder is logged and swallowed, never
// propagated — a telemetry write must never fail the request it rides on. The
// tenant is read from ctx (set by tenantAuth); an empty tenant (a pre-auth or
// direct-call context with no tenantAuth in front of it) records nothing.
func (h *handlers) recordTenantCall(ctx context.Context, tool string) {
	tenant := TenantFromContext(ctx)
	if tenant == "" || h.tenantCalls == nil {
		return
	}
	if err := h.tenantCalls.CountTenantCall(tenant, tool, time.Now()); err != nil {
		h.logger.Warn("tenant usage record failed",
			slog.String("tenant", tenant),
			slog.String("tool", tool),
			slog.String("error", err.Error()),
		)
	}
}

// withTenantTelemetry wraps an MCP tool handler so every call — success or
// tool error alike — bumps the calling tenant's per-tool counter (#0126)
// before the handler runs. Generic over each tool's distinct Args/Result
// types so every mcp.AddTool call site in New can share one wrapper.
func withTenantTelemetry[In, Out any](h *handlers, tool string, fn mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args In) (*mcp.CallToolResult, Out, error) {
		h.recordTenantCall(ctx, tool)
		return fn(ctx, req, args)
	}
}
