---
id: 0058
title: "Grow the gold set from daily-audit misses (ongoing)"
status: open
severity: low
group: 0034
depends_on: [0044]
forgejo: 148
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

Every daily-audit disagreement (Opus vs the 20B judge) becomes a new gold-set case → re-measure the prompt. The flywheel that makes the judge monotonically better. Ongoing process + the wiring to make it cheap.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §F4.

## Touches

`internal/judgeeval/gold.yaml`; a small intake from the #0044 audit output.

## Acceptance

- [ ] A documented process (and helper) turns an audit miss into a new gold case + a re-measure.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
