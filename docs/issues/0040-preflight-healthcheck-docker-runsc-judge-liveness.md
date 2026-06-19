---
id: 0040
title: "Preflight healthcheck (docker/runsc + judge liveness)"
status: open
severity: medium
group: 0034
depends_on: []
forgejo: 130
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The loop discovers a dead broker or unreachable judge only partway through. Probe `docker info` (+ runsc present) and a judge liveness ping before walking the corpus; abort cleanly if down.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §A3.

## Touches

`cmd/twiceshy/main.go` setup; a Ping/Healthy seam in `internal/repro` and `internal/judge`.

## Acceptance

- [ ] With docker stopped or the judge unreachable, the command aborts before processing any record.
- [ ] The abort names which preflight check failed.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
