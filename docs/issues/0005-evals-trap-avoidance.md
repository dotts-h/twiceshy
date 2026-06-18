---
id: 0005
title: Trap-avoidance eval suite — memory on/off regression for the store
status: open
severity: medium
group: 0008
depends_on: [0002]
forgejo:
links:
  adr: docs/adr/ADR-0001-architecture.md
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
The project's regression suite for the store itself (Phase 5): walk an agent
toward each recorded trap with memory **on vs off**, and score avoidance plus
steps/tokens. Publishable novelty — no published suite measures this
(ADR-0001 §8).

## Scope
- [ ] Harness: drive an agent toward each `trap`/`dead-end` record, memory on/off.
- [ ] Metrics: avoidance rate, steps-to-solution, tokens; per-record and aggregate.
- [ ] Wire into `make ci` (or a separate target) as the store's regression gate.
- [ ] Report the near-miss failure mode explicitly (does a related-but-wrong card hurt?).

## Notes
Depends on #0002 (push) and a non-trivial corpus (#0007) — needs records and an
injection path to evaluate end to end.
