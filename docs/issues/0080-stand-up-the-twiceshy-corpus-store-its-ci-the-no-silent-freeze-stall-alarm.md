---
id: 0080
title: Stand up the twiceshy-corpus store + its CI + the no-silent-freeze stall alarm
status: open
severity: high
group: 0076
depends_on: [0077]
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
ADR-0021 phase 2: create the corpus store from the snapshot (C0077), with its OWN CI (schema-validate + validated-scoped doctors, exp-0746) and a stall alarm that never swallows an auto-merge result. Not yet authoritative.
