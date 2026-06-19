---
id: 0041
title: "Production majority voting in promote"
status: open
severity: high
group: 0034
depends_on: []
forgejo: 131
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The judge is single-shot in production; gpt-oss:20b is non-deterministic at temp 0 on boundary cases (~0.7% single-shot false-approve, exp-0046). Judge each record repeat-N (default 3), promote on majority-approve only — measured 0% false-approve at repeat=5.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §F1.

## Touches

`internal/promote` / the promote judge call.

## Acceptance

- [ ] `promote` calls the judge N times per record and promotes on majority-approve only.
- [ ] N is configurable; default ≥3.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
