---
id: 0052
title: "Wire the Reaper at promote/adapt startup"
status: open
severity: medium
group: 0034
depends_on: []
forgejo: 142
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The Reaper exists but is never invoked by the loop, so a crashed run's gVisor containers/volumes accumulate. Call it before the corpus walk; document a periodic sweep.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §D2.

## Touches

`cmd/twiceshy/main.go` (call repro.NewReaper().Reap before the walk); `internal/repro/reaper.go`.

## Acceptance

- [ ] A crashed prior run's containers/volumes are swept before the next run starts.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
