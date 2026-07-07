// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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

// alphaReportDailyQuota is the per-token, per-UTC-day contribution quota for
// report_outcome and report_issue (#0128) — separate from record_experience's
// own, tighter quota and from tenantAuth's per-call rate quota.
const alphaReportDailyQuota = 25

// isAlphaTenant reports whether tenant is an untrusted alpha tok_ tenant
// (ADR-0030 phase 2) as opposed to "operator". Shared by every write-path
// hardening check added in #0128 so they all agree on the same predicate.
func isAlphaTenant(tenant string) bool {
	return strings.HasPrefix(tenant, "tok_")
}

// contributionQuota is the slice of the index the write-path contribution
// quota needs (ADR-0032): an atomic, fail-closed per-tenant, per-tool debit.
// Narrowed to an interface, like tenantCallRecorder/usageStore, so it is
// unit-testable with a stub and documents that — unlike tenant_usage
// telemetry — this store's errors DO gate the request.
type contributionQuota interface {
	CountContributionCall(tokenID, tool string, limit int, now time.Time) (int, bool, error)
}

// checkContributionQuota enforces the per-tenant, per-tool CONTRIBUTION quota
// (#0128) for the write-path tools (record_experience / report_outcome /
// report_issue) — separate from tenantAuth's per-call rate quota (#0125).
// Enforcement debits its own atomic, fail-closed counter via contribQuota
// (ADR-0032); the tenant_usage telemetry counter (#0126) only observes calls
// and never gates them. The operator tenant is never gated — this quota
// exists for untrusted alpha tok_ tenants only.
func (h *handlers) checkContributionQuota(ctx context.Context, tool string, limit int) error {
	tenant := TenantFromContext(ctx)
	if !isAlphaTenant(tenant) {
		return nil
	}
	if h.contribQuota == nil {
		return fmt.Errorf("checking %s contribution quota: no contribution-quota store wired", tool)
	}
	calls, allowed, err := h.contribQuota.CountContributionCall(tenant, tool, limit, time.Now())
	if err != nil {
		return fmt.Errorf("checking %s contribution quota: %w", tool, err)
	}
	if !allowed {
		return fmt.Errorf("%s daily contribution quota exceeded (%d/%d) for this token — try again tomorrow (UTC)", tool, calls, limit)
	}
	return nil
}
