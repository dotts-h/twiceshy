---
id: 0057
title: "Adaptive `-confirm` mode in judge-eval"
status: open
severity: low
group: 0034
depends_on: []
forgejo: 147
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

judge-eval repeats ALL cases uniformly; only the boundary cases flip. Add a mode that runs one pass, then re-samples only the cases that flipped (~3× cheaper re-runs at equal confidence).

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §F3.

## Touches

`internal/judgeeval` + the judge-eval subcommand.

## Acceptance

- [ ] `judge-eval -confirm` re-samples only flipped cases and reports the same headline confidence.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
