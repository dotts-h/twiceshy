---
id: 0050
title: "Validator desync guards (valid.until / demotion)"
status: open
severity: medium
group: 0034
depends_on: []
forgejo: 140
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

A manual reversal can silently desync: `record.Validate` does not reject a `validated` record whose `valid.until` is past, nor one still carrying a `provenance.demotion` block (→ staleness doctor re-flags it → validated↔stale flip-flop). Add both guards.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §C4.

## Touches

`internal/record/record.go` validateProvenance + tests.

## Acceptance

- [ ] A validated record with a past valid.until or a lingering demotion block fails validation.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
