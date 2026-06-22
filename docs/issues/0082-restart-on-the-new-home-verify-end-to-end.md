---
id: 0082
title: Restart on the new home + verify end-to-end
status: open
severity: high
group: 0076
depends_on: [0081]
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
ADR-0021 phases 5-6: re-enable the timers against the corpus store; verify a full import->quarantined->validate->served cycle, id-allocation across the move (no colliding exp-NNNN), the gold/eval suites against the fixture, and that the stall alarm fires on a synthetic red.
