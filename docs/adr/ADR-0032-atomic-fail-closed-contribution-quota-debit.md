# ADR-0032: Contribution quota is an atomic, fail-closed store debit, decoupled from telemetry

- **Status:** Accepted (2026-07-07; horia approved the 2026-07-07 architecture
  review's proposal; blocks #0128 write-path enablement).
- **Related:** ADR-0030 (public alpha; phase 2 opens the write tools this quota
  bounds), #0128 (the contribution quota this fixes), #0131 (finding 1 — the
  same fail-open-by-ordering class; finding 2 — the atomic-UPSERT pattern this
  reuses), #0126 (the tenant telemetry counter this disentangles), issue 0135.

## Context

#0128 added per-tool daily contribution quotas for untrusted alpha tok_
tenants (record_experience 10/day, report_outcome / report_issue 25/day) —
the bound on how fast one hostile token can push content at the moderation
pipeline. The implementation reads the `tenant_usage` counter
(`checkContributionQuota` → `TenantToolCallsToday`), which is written only by
`recordTenantCall` — the #0126 per-tenant telemetry bump whose contract is
explicitly best-effort: "log and swallow, never fail the request".

Two invariants collide on one table:

- **Telemetry must never gate** (the charter of `internal/telemetry` and of
  `recordTenantCall`'s hard-rule comment) — yet here it does.
- **Quota must gate** — yet its counter is written by a path that swallows
  errors, so a failing write (locked db, disk pressure: exactly what a hostile
  flood produces) makes the quota undercount and **fail open**.

On top of that, correctness depends on `withTenantTelemetry` having bumped the
counter BEFORE the handler reads it — a comment-enforced temporal coupling
across the MCP-wrapper layer and the handler body, the same class of invariant
that produced #0131 finding 1. And the read bypasses the package's
consumer-side-interface idiom, calling the concrete `*index.Index` directly.

## Decision

Enforcement gets its own storage and its own synchronous, atomic, fail-closed
debit; telemetry reverts to pure observation.

1. **New store method** `(*index.Index).CountContributionCall(tokenID, tool
   string, limit int, now time.Time) (calls int, allowed bool, err error)`,
   backed by a new `contribution_usage (token_id, day, tool, calls)` table
   (additive `CREATE TABLE IF NOT EXISTS` in `Open`, the house migration
   pattern). The cap check and the increment live in ONE conditional UPSERT —
   the exact pattern `CountTokenCall` proved under -race (#0131 finding 2).
   A call at or over `limit` leaves the row unchanged and returns
   `allowed=false`.
2. **A narrow consumer-side interface** in `internal/server`
   (`contributionQuota`), wired from `Config.Index` like `usageStore` /
   `tenantCallRecorder`. `checkContributionQuota` calls it synchronously;
   **any store error rejects the write** — untrusted writes fail closed, the
   opposite default from read-path telemetry.
3. **`tenant_usage` returns to pure telemetry.** `TenantToolCallsToday` loses
   its enforcement consumer; the bump-before-check comments go away. The
   quota's semantics become debit-on-attempt: a call rejected later in the
   handler (size cap, secret gate) still consumed one — conservative in the
   safe direction for a hostile-input surface.

The operator tenant remains exempt (the quota exists for untrusted tok_
tenants only), unchanged from #0128.

## Options considered

- **Keep reading `tenant_usage` but propagate errors (fail closed) —
  rejected:** closes the fail-open but keeps the temporal coupling, the mixed
  telemetry/enforcement charter on one table, and the double duty of a counter
  that also counts read-tool calls.
- **Reuse `token_usage` / `CountTokenCall` — rejected:** wrong granularity
  (per-token-per-day, no tool dimension); the write-path quota is per-tool by
  design (#0128).
- **Enforcement-owned table + atomic debit (chosen):** one writer, one
  reader, one statement; each table has exactly one charter.

## Consequences

- Under store failure the alpha write path rejects instead of silently
  unbounding — an outage degrades to "contributions unavailable", never to
  "quota off". Acceptable: these are untrusted writes.
- One fewer comment-enforced ordering invariant in the request path; the
  `withTenantTelemetry`-bumps-first assumption stops being load-bearing.
- `tenant_usage` counts every attempt for observability, `contribution_usage`
  counts enforcement debits; dashboards keep reading the former.
- Small additive schema migration; the table is durable-but-cheap state in the
  index db (same posture as `token_usage`; the ADR-0034 registry question
  covers all three tenant tables together).
