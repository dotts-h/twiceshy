---
id: 0039
title: "Single-flight lock around promote/adapt"
status: closed
severity: high
group: 0034
depends_on: []
forgejo: 129
links:
  adr: ADR-0013
  prs: [154]
  issues: []
  regression:
assets: []
---

## Summary

No lock today — two overlapping runs (a slow run + the next cron tick, or a manual run during the timer) both load the corpus and can double-write. Add a flock on a corpus-local lockfile.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §A2.

## Touches

`cmd/twiceshy/main.go` (runPromote/runAdapt) or the nightly driver; a small lock helper.

## Acceptance

- [x] A second invocation while one holds the lock exits non-zero with a clear message.
- [x] A test covers the contention path.
- [x] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
