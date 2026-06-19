---
id: 0030
title: Usage signal — retrieval increments provenance.usage (unblocks D4 + the reinforce/decay signal)
status: open
severity: medium
group: 0027
depends_on: []
forgejo:
links:
  adr: ADR-0013
  prs: []
  issues: [0004]
  regression:
assets: []
---

## Summary

Give the closed loop a reinforcement signal distinct from execution: retrieval
increments `provenance.usage` (`retrieved`, `last_hit`; `confirmed_helpful` is set
by a positive `report_outcome`, #0031). ADR-0010 explicitly deferred the **D4
lifecycle** doctor "until retrieval increments `provenance.usage`" — this is that
substrate.

## Touches

`internal/index`/`internal/server` (the read path) + `internal/record` (the usage
block already exists). Counters are a per-record delta, written via the same
trusted persistence path (ADR-0008) — never a hot-path synchronous write that slows
retrieval; batch/async so the embedding-free read path stays cheap.

## Acceptance

- [ ] A served record's `retrieved` + `last_hit` advance; updates are durable and
  don't block the read path's latency budget.
- [ ] `confirmed_helpful` is settable by a positive outcome report (#0031).
- [ ] No double-count / race under concurrent reads (counters are monotonic).
- [ ] Test-first; `make ci` green.

## Notes

Decouples "used a lot" from "proven" — a heavily-retrieved card that starts drawing
negative reports (#0031/#0032) is exactly what D4 should decay. Independent lane
(disjoint seam from the judge/promotion work).
