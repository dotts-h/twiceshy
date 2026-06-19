---
id: 0046
title: "Unify broker reaper logging to slog"
status: open
severity: low
group: 0034
depends_on: [0035]
forgejo: 136
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

`internal/repro/broker.go` reaper uses bare `log.Printf` — a third logging style. Convert to the project slog logger with structured fields, so the nightly operator has one format to grep/ship.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §B5.

## Touches

`internal/repro/broker.go` reaper log sites.

## Acceptance

- [ ] Reaper events are structured slog; no bare log.Printf remains on the loop path.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
