# ADR-0014: Share the judge-eval result-aggregation logic between Run and RunConfirm

- **Status:** Accepted (deciders: claude, during the post-epic-0034 architecture
  pass). Low-risk structural cleanup; no behavior change.
- **Related:** #0057 (RunConfirm, the adaptive `-confirm` mode that introduced the
  duplication), ADR-0013 §F (the judge eval is how the prompt's safety is measured).

## Context

`internal/judgeeval` scores a judge against the gold set. There are two entry
points: `Run` (uniform — every case sampled `repeat` times) and `RunConfirm`
(adaptive — sample `base`, then top up only the flipped/boundary cases, #0057).

When `RunConfirm` was added it copied `Run`'s per-case accumulation (RejectCases /
ApproveCases / Errors / FalseApproves / FalseRejects / Correct / ChecksCaught /
Flips / Outcomes) **and** the derived-rate computation (FalseApproveRate,
FalseRejectRate, Accuracy, CheckRecall) verbatim — ~40 duplicated lines across the
two functions.

This is a **drift hazard on the load-bearing metric.** `FalseApproveRate` is the
fail-unsafe headline the judge prompt is chosen on. If a future change touched the
aggregation in one function and not the other, `Run` and `RunConfirm` would report
the *same* gold set differently — and the whole point of `-confirm` is that it
reports the *same headline* at fewer calls. Two copies of the metric math quietly
make that guarantee unenforceable.

## Options considered

1. **Leave it.** Two copies, kept in sync by hand. Cheapest now; the exact way the
   "same headline" guarantee rots later. Rejected.
2. **Extract shared `(*Result).tally(c, o)` + `(*Result).finalize()` methods** that
   both functions call. One definition of the metric; the two paths differ only in
   *how they sample* (uniform vs adaptive), which is their real distinction.
3. Collapse `Run` into `RunConfirm(base==total)`. More invasive, changes `Run`'s
   signature/JudgeCalls semantics, and couples the uniform path to the adaptive
   one's control flow. Rejected as over-reach for a cleanup.

## Decision

Option 2. `tally` folds one scored `Outcome` into the running `Result`; `finalize`
computes the derived rates once after the loop. `Run` and `RunConfirm` now contain
only their sampling difference and call the shared aggregation. The boundary is:
**sampling strategy is per-function; metric aggregation is one shared definition.**

## Consequences

- The "same headline at ~3× fewer calls" guarantee (#0057) is now *structural* —
  both paths compute FalseApproveRate from the same code; they cannot drift.
- `Run`/`RunConfirm` shrink to their essential difference (sampling), improving
  readability.
- The existing `Run` and `RunConfirm` tests are the regression guard: the refactor
  is behavior-preserving, proven by `make ci` staying green with no test changes.
- Future metrics added to the eval are added once (in `tally`/`finalize`) and apply
  to both paths automatically.
