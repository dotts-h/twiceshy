---
id: 0084
title: Promote throughput — decouple the throttle from the anomaly halt, and stop re-judging the held backlog every run (hold cooldown)
status: closed
severity: high
group: 0034
depends_on: []
forgejo: 457
links:
  adr: docs/adr/ADR-0022-promote-throughput-and-hold-cooldown.md
  prs: []
  issues: [0033, 0043, 0085]
  regression:
assets: []
---

## Summary
Two conflated defects bounded promote throughput and made it degrade over time:

1. **`MaxActions` (anomaly alert threshold, default 25) doubled as the throughput
   throttle.** The promote loop halts the moment `actions > MaxActions`, so every
   run promoted exactly **26** then halted with `errAnomalyHalt` (exit 3) — even
   with time + thousands of eligible candidates left. Because 25 sits *below* the
   natural batch size, **every** batch was (a) capped at 26 and (b) falsely marked
   ANOMALY ("will NOT auto-merge"). Observed: 17 consecutive nightly batches each
   promoted 26 and each carried the anomaly marker.

2. **No hold cooldown — the held backlog re-judges itself every run.**
   `promote.Promotable` has no memory of prior holds, so a panel-declined record
   stays `quarantined` AND stays eligible → it gets a full `gpt-oss:20b`+frontier
   panel call again on the next scheduled run (~every 30 min), forever. The `held:
   99` in run #22 is that growing graveyard. The 26-cap merely *masked* the cost by
   halting early; raising the cap would make each run re-judge an ever-larger pile —
   genuinely "slower and slower" (operator's intuition; confirmed in code + an
   `ask-agy` duck).

## Fix
- **Decouple (ADR-0022).** New `-max-promotions` **throughput cap**: a *clean* stop
  (exit 0, mergeable batch, "re-run to continue"), distinct from the anomaly halt.
  `MaxActions` is now the **unbounded-mode backstop only** — `Budget.Anomalous()`
  returns false whenever a throughput cap is set (the cap governs; a full batch is
  never mis-flagged). `internal/guard` grows `MaxPromotions` + `Capped()`.
- **Hold cooldown.** New `-hold-cooldown` (default **7d**): an engine-maintained
  ledger `<corpus>/runs/promote.holds.json` (sibling of the run journals) records
  when each record was last held; the promote walk skips records still inside the
  window. Operational state on the `-corpus` path — NOT an experience record, so the
  corpus's data-only validation ignores it and engine CI (frozen fixtures, #0079)
  never loads it. Rides the same `-corpus` seam (#0081) the journals already use.
- **Driver.** `scheduled-validate.sh` passes `-max-promotions`/`-max-actions`/
  `-hold-cooldown` from env (`TWICESHY_MAX_PROMOTIONS` [default 0 = off],
  `TWICESHY_MAX_ACTIONS` [25], `TWICESHY_HOLD_COOLDOWN` [168h]). Cap defaults **off**
  so the binary's behaviour is unchanged until an operator opts in at deploy time —
  enable the cap only **after** the cooldown is live, or each capped run still
  re-judges the held backlog.

## Acceptance
- [x] A full batch stops **cleanly** at `-max-promotions` (exit 0, no ANOMALY marker,
      auto-mergeable) — `TestPromoteCorpus_ThroughputCapStopsCleanly`,
      `TestBudget_CapStopsBeforeAnomaly`, `TestBudget_CapDisablesCountAnomaly`.
- [x] The anomaly halt still fires in **unbounded** mode (compromised-judge backstop)
      — `TestBudget_AnomalyBackstopWhenUncapped`,
      `TestPromoteCorpus_AnomalyOnFinalActionStillHalts`.
- [x] A record held within the cooldown is **not** re-judged next run; the window
      expiring re-enables it; a promotion clears it — `TestHoldLedger_*`,
      `TestFilterCooldown_DropsHeldKeepsRest`, `TestNoteOutcomes_*`.
- [x] `make ci` green (lint 0, race tests, coverage ≥ floor).

## Known tradeoff (tracked: #0085)
With a throughput cap set, the **count**-based anomaly is moot by construction (the
cap stops a normal run first). The compromised-judge defense in capped mode is the
veto window + per-record gate/attestation + daily audit. A promotion-**rate** anomaly
that survives a cap (e.g. "approval fraction ≫ baseline") is the proper successor —
**#0085**.
