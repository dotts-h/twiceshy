---
id: 0042
title: "report_outcome → corpus intake (so adapt has nightly input)"
status: closed
severity: high
group: 0034
depends_on: []
forgejo: 132
links:
  adr: ADR-0013
  prs: [157]
  issues: []
  regression:
assets: []
---

## Summary

`report_outcome` returns markdown to the caller and never lands on disk, so `adapt` has no nightly input without a human paste-commit-merge — the break in the auto-adapt chain. Add a path that materializes queued counter-records into experience/ automatically (e.g. `twiceshy intake-reports`).

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §E1.

## Touches

`internal/server/report.go` (queue) + a new intake CLI; the nightly driver (#0043).

## Acceptance

- [x] A reported outcome appears as a quarantined counter-record the next run's `adapt` processes.
- [x] No human paste step is required.
- [x] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
