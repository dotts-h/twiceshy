---
id: 0055
title: "Materialize the usage signal into provenance.usage"
status: open
severity: medium
group: 0034
depends_on: []
forgejo: 145
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

Usage (retrieved/confirmed_helpful/last_hit) is write-only on the serve host's SQLite; the loop loads markdown and sees usage:0, so it can't reinforce. Flush usage back into `provenance.usage` (a delta-only doctor or a nightly flush the driver commits).

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §E2.

## Touches

`internal/index/usage.go` (materialize); a doctor or driver flush step.

## Acceptance

- [ ] The committed corpus reflects real usage counters the loop and audit can read.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
