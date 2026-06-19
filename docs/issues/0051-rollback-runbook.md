---
id: 0051
title: "Rollback runbook"
status: open
severity: medium
group: 0034
depends_on: [0043, 0048]
forgejo: 141
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

Document the recovery procedure: engage `TWICESHY_PAUSE`; close the open promotion PR to veto; `git revert` the night's commit to batch-roll-back; the #0048 command to restore a record. Cross-link ADR-0013 §2 + the SCHEMA lifecycle.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §C5.

## Touches

`docs/` runbook.

## Acceptance

- [ ] A runbook covers veto, batch-rollback, and single-record restore with exact commands.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
