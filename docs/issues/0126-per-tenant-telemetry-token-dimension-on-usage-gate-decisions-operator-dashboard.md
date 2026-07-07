---
id: 0126
title: Per-tenant telemetry: token dimension on usage + gate decisions, operator dashboard
status: closed
severity: medium
group: 0124
depends_on: []
forgejo: 527
links:
  adr:
  prs: [516]
  issues: []
  regression:
assets: []
---

## Summary

Add the tenant (token id) dimension to usage counters and gate-decision/tool-call
telemetry, and build the operator dashboard on top: per-tenant calls, top
records served, quota consumption, corpus stats, promotion liveness (#0122).
Digest (#0116) gains a per-tenant section. Dashboard can be a single static
HTML page fed by a read-only JSON endpoint — no framework budget (CONVENTIONS
dependency budget applies).

## Notes

Alarm on floods and on throughput-zero, not just errors (the #0122/#0123
lesson). Per-tenant data is also the future billing meter — keep the schema
aggregation-friendly.

## Close-out (2026-07-06, PR #516)

Shipped: the `tenant_usage` table, the `/statz` operator endpoint, and the
web/dashboard page. The digest (#0116) per-tenant section is deferred — not
built in this PR; tracked as follow-up work, not blocking the alpha launch.
