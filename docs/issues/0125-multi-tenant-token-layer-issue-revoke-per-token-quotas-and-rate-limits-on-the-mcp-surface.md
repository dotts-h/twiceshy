---
id: 0125
title: Multi-tenant token layer: issue/revoke, per-token quotas and rate limits on the MCP surface
status: closed
severity: medium
group: 0124
depends_on: []
forgejo:
links:
  adr:
  prs: [511]
  issues: []
  regression:
assets: []
---

## Summary

Replace the single shared bearer token (`internal/server/server.go` bearerAuth)
with a token table: issue/revoke, per-token quotas (daily call budget) and rate
limits (per-minute), enforced on every MCP tool call. Token id becomes the
tenant key that #0126 telemetry and #0128 trust tiers hang off. Admin surface
can be a CLI subcommand (`twiceshy token issue|revoke|list`) — no web admin
needed for alpha. SQLite table in the existing DB; keep the hot path
embedding-free and cheap (one indexed lookup).

## Notes

Fail-safe direction: unknown/revoked/over-quota token → 401/429 with a stable
error shape; never fall through to the shared-secret path. The legacy single
token remains only for the private LAN instance.

## Close-out (2026-07-06, PR #511)

Shipped: the token table (issue/revoke, per-token daily quota and per-minute
rate limit) enforced on every MCP tool call; the tenant-context seam token id
becomes the tenant key #0126 telemetry and #0128 trust tiers hang off; and the
`twiceshy token issue|revoke|list` CLI. Review follow-ups from the PR are
tracked in #0131.
