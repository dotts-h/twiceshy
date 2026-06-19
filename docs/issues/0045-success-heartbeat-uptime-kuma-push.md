---
id: 0045
title: "Success heartbeat (Uptime-Kuma push)"
status: closed
severity: medium
group: 0034
depends_on: [0043]
forgejo: 135
links:
  adr: ADR-0013
  prs: [160]
  issues: []
  regression:
assets: []
---

## Summary

A silently-skipped or misconfigured nightly run (no run = no diff = looks like a quiet night) is undetectable. POST to a configurable Uptime-Kuma push URL (`TWICESHY_HEARTBEAT_URL`, env-gated) on clean completion.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §B4.

## Touches

end of runPromote/runAdapt; reuse the #0038 notify seam.

## Acceptance

- [x] A clean run pings the heartbeat URL; a skipped run is detectable as a missed ping.
- [x] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
