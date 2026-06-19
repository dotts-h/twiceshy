---
id: 0053
title: "Fail-safe verification tests (broker outage / poison record)"
status: closed
severity: high
group: 0034
depends_on: []
forgejo: 143
links:
  adr: ADR-0013
  prs: [169]
  issues: []
  regression:
assets: []
---

## Summary

Under-covered failure modes need guards: (a) broker/docker outage → attestation error → records-before-it persisted, run aborts non-zero, nothing bad promoted; (b) a poison/unparseable record is skipped, not fatal to the whole run.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §D3.

## Touches

`cmd/twiceshy/promote_test.go`, `adapt_test.go` (broker stub returning start errors + a poison fixture).

## Acceptance

- [x] A broker-outage test proves nothing bad promotes and the run exits non-zero.
- [x] A poison/unparseable record does not kill the whole run.
- [x] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
