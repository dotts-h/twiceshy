# ADR-0022: Promote throughput — a clean cap decoupled from the anomaly halt, plus a hold cooldown

- **Status:** Accepted (2026-06-23) — decider: **horia** (directed the throughput
  fix; chose the engine-maintained ledger and the 7-day cooldown); claude proposed
  and authored. Reasoning was pressure-tested with an `ask-agy` (Gemini Pro) duck,
  which surfaced the hold-accumulation failure mode that flipped the design.
- **Related:** [ADR-0013](ADR-0013-closed-loop-autonomous-validation.md) §7/§D1 (the
  guardrails — emergency stop / anomaly / budget — this refines); [ADR-0016](ADR-0016-advisory-class-panel-promotion.md)
  / [ADR-0020](ADR-0020-prose-class-panel-promotion.md) (the panels whose judge calls
  this conserves); [ADR-0021](ADR-0021-decouple-corpus-as-a-data-product.md) / [ADR-0005](ADR-0005-stable-seams.md)
  (the `-corpus` seam the ledger rides); issues **#0084** (this change), **#0085**
  (the rate-anomaly successor), **#0033/#0043** (guardrails / nightly driver).

## Context

The autonomous promote loop (`twiceshy promote`, run ~every 30 min by
`scheduled-validate.sh`) walks the quarantined backlog and, per eligible record,
runs a `gpt-oss:20b` + frontier judge panel; an approval flips `quarantined →
validated`. Two defects bounded and degraded it:

1. **`MaxActions` (the anomaly alert threshold, default 25) was also the throttle.**
   The loop halts the instant `actions > MaxActions`, returning `errAnomalyHalt`
   (exit 3). With 25 below the natural batch size, every run promoted exactly **26**
   then halted — and every batch PR was marked ANOMALY / "will NOT auto-merge",
   training the operator to ignore the signal (17 consecutive nights).

2. **No hold cooldown.** `promote.Promotable` has no memory of prior holds, so a
   panel-declined record stays eligible and is re-judged — a full panel call — every
   run, forever. The held pile (`held: 99` and growing) re-judges itself; the 26-cap
   only masked the cost. Raising the cap naively makes each run re-judge an
   ever-larger graveyard (the "slower and slower" the operator predicted).

The count-anomaly and the throttle are **different concerns** wearing one number:
"how big is a normal batch" vs. "how large a spike means the judge is compromised."

## Decision

**1. Decouple throughput from anomaly.** Add `MaxPromotions` (`-max-promotions`): a
**clean** per-run ceiling — the loop stops with exit 0, a mergeable batch, and a
"re-run to continue" notice. `MaxActions` becomes the **unbounded-mode backstop
only**: `Budget.Anomalous()` returns false whenever `MaxPromotions > 0`, because a
capped run stops at the cap before any count could mean "spike." Ordering in the
loop: check `Capped()` (clean stop) *before* `Anomalous()` (halt).

**2. Hold cooldown.** Add `-hold-cooldown` (default **7 days**). An engine-maintained
ledger at `<corpus>/runs/promote.holds.json` maps record-id → last-held time; the
promote walk skips records still inside the window (a cheap pre-filter), folds each
run's held/promoted outcomes back in, and prunes expired entries on save.

**3. Where the ledger lives — the `-corpus` path, not the engine repo.** The ledger
is **operational state**, a sibling of the existing `runs/promote.journal.json`. It
is **not** an experience record, so the corpus's data-only record validation ignores
it (as it does `runs/` today), and engine CI never loads it (frozen fixtures, #0079).
This does **not** re-couple engine and corpus: the coupling that broke CI was *test
code asserting on live corpus content*; this is the engine writing runtime state onto
the `-corpus` dir it is handed (#0081) — exactly what the journals already do.

## Options considered

- **Throttle: raise `MaxActions`.** Rejected — it conflates the two concerns and,
  without the cooldown, still degrades as the held pile grows.
- **Cooldown store: a normative record field** (`last_held_at` in frontmatter).
  Rejected — mutates the record on decline (today a hold leaves the record untouched)
  and grows the normative SCHEMA for scheduling state.
- **Cooldown store: derive from `runs/*-promote.json` manifests.** Rejected — couples
  the engine to the driver's manifest layout + retention.
- **Cooldown store: engine-maintained ledger under `runs/` (chosen).** No schema
  change, decoupled, auditable, committed with the batch.

## Consequences

- **Positive.** Throughput is an explicit, tunable knob; a normal batch stops cleanly
  and auto-merges (no false anomaly). The held backlog stops re-judging itself —
  large reduction in wasted panel calls, and per-run cost no longer grows with the
  graveyard. Conserves frontier-judge quota.
- **Negative / tradeoff.** With a cap set, the **count**-anomaly is moot by
  construction; the compromised-judge defense in capped mode is the veto window +
  per-record gate/attestation + daily audit. The proper successor — a promotion-**rate**
  anomaly that survives a cap — is **#0085**.
- **Rollout.** The driver defaults the cap **off** (`TWICESHY_MAX_PROMOTIONS=0`) so
  behaviour is unchanged until an operator opts in. Enable the cap only **after** the
  cooldown is live, else a capped run still re-judges the held backlog.
