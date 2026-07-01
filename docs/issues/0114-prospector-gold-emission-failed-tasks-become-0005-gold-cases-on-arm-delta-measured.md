---
id: 0114
title: Prospector gold emission: failed tasks become #0005 gold cases; ON-arm delta measured
status: closed
severity: medium
group: 0112
depends_on: [0113]
forgejo: 493
links:
  adr: docs/adr/ADR-0029-model-hard-trap-prospector.md
  prs: [488]
  issues: [0112, 0113, 0005]
  regression:
assets: []
---

## Summary
Model-hard cases — records whose #0113 OFF-arm run failed — are appended,
deduped by `TrapID`, to a new prospect-gold file in `internal/agenteval`,
consumed alongside `GoldTasks()` (`internal/agenteval/agenteval.go:131`) by
the live #0005 eval. Each emitted case carries its measured ON-arm outcome, so
the gold set records the delta an experience card actually produced, not a
guess about what a model might fail. A model-hard record whose ON arm ALSO
fails (the card exists but doesn't help) is the most interesting class and is
reported distinctly — it is a card-quality lead, not folded silently into the
same bucket as an ON-arm pass.

## Repro
1. Run the prospector (#0113) against a record it drafts a valid task for; the
   OFF-arm run hits the trap.
Expected: the record's case is appended to the prospect-gold file with its
measured ON-arm result recorded (pass or still-fails), deduped against any
prior entry for the same `TrapID`.
Actual: no prospect-gold file exists; #0005's gold set has no mechanism to grow
from measured failures.

## Evidence
- `internal/agenteval/agenteval.go:131` (`GoldTasks`) is the existing static
  gold set the live eval reads today (3 cases, `gold_tasks_test.go:13`) — the
  prospect-gold file is consumed alongside it, not instead of it.
- #0113 is the sole producer: only its OFF-arm failures (model-hard cases) are
  eligible for emission here.

## Acceptance
- Model-hard OFF-arm failures are appended to a prospect-gold file in
  `internal/agenteval`, deduped by `TrapID` against prior entries.
- Each emitted case records its ON-arm outcome (avoided / still-fails)
  alongside the OFF-arm failure that qualified it.
- The live #0005 eval consumes the prospect-gold file alongside `GoldTasks()`
  without changing the existing 3 static cases.
- Records whose ON arm also fails are counted and reported as a distinct class
  ("card exists but doesn't help"), not merged into the plain model-hard count.

## Notes
Depends on #0113 (the OFF-arm run and verdict this issue consumes). The
ON-arm-also-fails class is explicitly not auto-demoted or auto-flagged as
poor quality here — ADR-0029's consequences section defers that decision;
this issue only ensures the signal is visible in the report.
