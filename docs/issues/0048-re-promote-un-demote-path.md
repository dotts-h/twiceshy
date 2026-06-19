---
id: 0048
title: "Re-promote / un-demote path"
status: open
severity: high
group: 0034
depends_on: []
forgejo: 138
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The only un-demote today is a hand-edit. A wrong auto-demote (sandbox≠prod, flaky counter, compromised judge) needs a clean reversal. Add a command that takes a stale/disputed record back through the gate+judge and, on a hold, restores `validated` — clearing `valid.until` and the `provenance.demotion` block.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §C2.

## Touches

`internal/promote` (allow a re-promote entry from stale/disputed); `cmd/twiceshy/main.go`.

## Acceptance

- [ ] A demoted record is restored by one command; valid.until/demotion are unwound.
- [ ] Test-first.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
