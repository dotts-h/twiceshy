---
id: 0135
title: Atomic fail-closed contribution-quota debit — decouple write-path quota enforcement from best-effort tenant telemetry
status: closed
severity: high
group: 0124
depends_on: []
forgejo: 578
links:
  adr: ADR-0032
  prs: [542]
  issues: [0128, 0131]
  regression:
assets: []
---

## Summary

The #0128 per-tool contribution quota for alpha tenants is enforced by reading
the `tenant_usage` telemetry counter (`checkContributionQuota` →
`h.ix.TenantToolCallsToday`, `internal/server/tenant_usage.go:74-87`), whose
only writer is the best-effort `recordTenantCall` path — a write whose stated
contract is "log and swallow, never influence the request". Three defects,
found by the 2026-07-07 pre-enablement architecture review:

1. **Fails open.** A swallowed `CountTenantCall` error (locked db, disk
   pressure — the conditions a hostile flood creates) undercounts, and the
   daily poisoning-rate bound silently stops binding.
2. **Comment-enforced temporal coupling** (#0131 class): correctness depends
   on `withTenantTelemetry` bumping the counter BEFORE the handler reads it —
   narrated in comments at `tenant_usage.go:69-73` and
   `internal/index/tokens.go:219-225`, checked nowhere.
3. **Seam bypass.** The read goes through the concrete `*index.Index`, the
   only tenant-layer capability not behind a consumer-side interface.

## Fix (ADR-0032)

Give the contribution quota its own atomic, fail-closed debit:
`CountContributionCall(tokenID, tool, limit, now)` on `internal/index`, using
the conditional-UPSERT pattern `CountTokenCall` already proved (#0131 finding
2), against a new enforcement-owned `contribution_usage` table. The server
consumes it through a narrow interface; an error REJECTS the write (untrusted
writes fail closed, the opposite default from read-path telemetry).
`tenant_usage` reverts to pure best-effort telemetry.

## Repro
1. Make `CountTenantCall` fail (e.g. stub recorder erroring) while an alpha
   tok_ tenant calls `record_experience` 11+ times in one UTC day.
Expected: calls past the 10/day quota are rejected.
Actual (before fix): every call is admitted — the counter never advanced, the
quota check reads 0.

## Evidence

- `internal/server/tenant_usage.go:23-27` — "a telemetry write must never fail
  the request it rides on" vs the same counter gating requests at :74-87.
- `internal/index/tokens.go:206` — "callers must log and continue on error,
  never fail the request over it".
- Architecture review 2026-07-07 (session), finding B.

## Notes

Shipped with a guarding test per TDD; blocks #0128 write-path enablement for
alpha tenants alongside 0136 (ADR-0031).
