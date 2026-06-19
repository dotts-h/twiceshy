---
id: 0036
title: "`-json` run manifest for promote/adapt"
status: closed
severity: high
group: 0034
depends_on: [0035]
forgejo: 126
links:
  adr: ADR-0013
  prs: [151]
  issues: []
  regression:
assets: []
---

## Summary

No machine-readable outcome exists for the loop-mutating commands, so the morning review / daily audit must scrape stdout. Add a `-json` outcome to promote/adapt (and ingest): the stats struct + per-record actions (id, from_status, to_status, judge_model, judge_decision, reproduced_under, reason). This is the artifact the daily audit reads.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §B2.

## Touches

`cmd/twiceshy/main.go` runPromote/runAdapt; a small struct in `internal/promote` or a new package.

## Acceptance

- [x] `promote -json` emits valid JSON listing every record's transition.
- [x] The daily audit (#0044) can consume it without scraping stdout.
- [x] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
