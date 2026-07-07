---
id: 0137
title: Tenant registry is not derived state — Rebuild-preservation test, contract wording, runbook backup note (ADR-0034 phase 1)
status: open
severity: medium
group: 0124
depends_on: []
forgejo:
links:
  adr: ADR-0034
  prs: []
  issues: [0125, 0135]
  regression:
assets: []
---

## Summary
The four tenant tables (`tokens`, `token_usage`, `tenant_usage`, `contribution_usage`) live in the database file that ADR-0001 documents as derived and rebuildable. Because tokens are hash-only credentials, they are irrecoverable by design. The Rebuild-preserves invariant was accidental (untested) and the runbook backup text was stale.

## Repro
1. Delete the database file (`twiceshy.db`) of a tenant-serving deployment.
2. Reindex the corpus using `twiceshy index`.

Expected:
Per old documentation, a safe rebuild of the index cache occurs.

Actual:
Every issued token is revoked irrecoverably.

## Evidence

- `internal/index/index.go:413` — Rebuild's DELETE set omits the tenant tables (previously accidental, now pinned by `TestRebuildPreservesTenantRegistry`).
- CONTRACTS.md called usage counters "the one non-derived state" — false since #0125/#0126/ADR-0032.
- Architecture review 2026-07-07 (session), finding D.

## Notes
- Phase 1 of ADR-0034 is implemented.
- Phase 2 (physical split of the tenant registry into its own SQLite database file) is deferred with trigger conditions recorded in the ADR.
