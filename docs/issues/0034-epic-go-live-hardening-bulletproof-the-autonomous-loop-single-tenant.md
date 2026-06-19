---
id: 0034
title: "Epic: Go-live hardening — bulletproof the autonomous loop (single-tenant)"
status: closed
severity: high
group: 
depends_on: []
forgejo: 124
links:
  adr: ADR-0013
  prs: []
  issues: [0010]
  regression:
assets: []
---

## Summary

Take the autonomous promote/demote loop from "works when run by hand" to "runs unattended nightly, is fully observable, and every action is reversible" — **before** any public/dashboard/multi-tenant work. The full plan with per-item acceptance criteria is `docs/GO_LIVE_HARDENING_PLAN.md` (grounded in a 5-agent code audit, 2026-06-19). Keystone gap: the loop is not yet autonomous/observable/reversible, and ADR-0013 §2's PR/soak/veto window is not implemented in code.

## Children

MVP critical path (do in dependency order): #0035 #0036 #0037 #0038 #0039 #0040 #0041 #0042 #0043 #0044. Hardening tier: #0045–#0058. Public/auth/multi-tenant are DEFERRED to #0010.

## Acceptance

- [ ] Every MVP child (#0035–#0044) landed; a first instrumented overnight run produced a committed run manifest + a veto-window PR, and the daily audit reviewed it.
- [ ] Hardening children (#0045–#0058) landed or consciously deferred.
- [ ] `docs/GO_LIVE_HARDENING_PLAN.md` reconciled with what shipped.

## Notes

Plan of record: `docs/GO_LIVE_HARDENING_PLAN.md` (grounded in a 5-agent code audit, 2026-06-19). Public/dashboard/auth/multi-tenant are out of scope here — see epic #0010.
