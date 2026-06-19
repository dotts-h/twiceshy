---
id: 0047
title: "Judge latency + verdict-distribution metrics"
status: closed
severity: medium
group: 0034
depends_on: [0036]
forgejo: 137
links:
  adr: ADR-0013
  prs: [162]
  issues: []
  regression:
assets: []
---

## Summary

Nothing times the judge call or aggregates approve/reject ratios, so a degrading/hung judge and the subtler 'approves a higher fraction' compromise are invisible. Time the call and add the verdict distribution to the run summary.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §B6.

## Touches

`internal/judge/model.go` (or a timing wrapper); promote/adapt stats.

## Acceptance

- [x] The run summary reports judge p50/p95 latency and the approve/reject/held ratio.
- [x] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
