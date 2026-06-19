---
id: 0056
title: "Positive-outcome MCP path (confirmed_helpful)"
status: open
severity: medium
group: 0034
depends_on: []
forgejo: 146
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The loop can only ever demote/dispute — never reinforce; `confirmed_helpful` is permanently 0. Add an MCP 'this lesson worked' tool (and a `ConfirmHelpful` caller) so the §4 decay/reinforce balance exists.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §E3.

## Touches

`internal/server` (a confirm tool); `internal/index/usage.go`.

## Acceptance

- [ ] A positive report increments `confirmed_helpful`; a test covers it.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
