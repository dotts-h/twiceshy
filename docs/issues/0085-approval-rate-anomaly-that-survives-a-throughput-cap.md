---
id: 0085
title: Promotion-rate anomaly that survives a throughput cap (successor to the count-anomaly retired in capped mode)
status: closed
severity: medium
group: 0034
depends_on: [0084]
forgejo: 458
links:
  adr: docs/adr/ADR-0022-promote-throughput-and-hold-cooldown.md
  prs: []
  issues: [0033, 0084]
  regression:
assets: []
---

## Summary
#0084 decoupled the throughput throttle (`-max-promotions`, a clean stop) from the
anomaly halt (`MaxActions`, a count). A side effect: when a throughput cap is set,
the **count**-based anomaly cannot fire — a normal run always stops at the cap first,
so `Budget.Anomalous()` is short-circuited to false in capped mode (see
`internal/guard/guard.go`). The compromised-judge signal "the judge suddenly approves
everything" is therefore unguarded *as a counter* when running capped (which is the
intended production mode).

In capped mode the residual defenses still hold — the veto window (soak + a human can
close the batch PR), the per-record gate + attestation, and the daily spot-audit — but
the automated spike detector is gone.

## Proposal
Replace the count-anomaly with an **approval-RATE** anomaly that is meaningful under a
cap: flag a run whose `promoted / judged` fraction exceeds a baseline (with a minimum
sample so a tiny run of 3/3 isn't flagged). A compromised judge approving ~everything
shows up as a rate spike regardless of the cap. Wire it as the halt/alert that
`MaxActions` used to be, gated on a minimum number of judged records.

## Acceptance
- [x] A run with an abnormally high approve-rate over a minimum sample halts/alerts
      even when `-max-promotions` is set. *(`guard.Budget.RateAnomalous()` does not
      short-circuit on a cap, unlike `Anomalous()`; folded into the post-loop anomaly
      halt in both promote and adapt. Guard:
      `TestBudget_RateAnomalyFiresUnderCap`.)*
- [x] A normal mixed run (most records held) does not trip it. *(low promoted/judged
      fraction → quiet. Guard: `TestBudget_RateAnomalyQuietOnNormalRun`.)*
- [x] Baseline + minimum-sample are flag/env tunable; defaults documented in ADR-0022.
      *(`-max-action-rate` (default 0 = off) + `-min-sample` (default 10) on promote and
      adapt; documented in ADR-0022 §Update. Guards: `TestBudget_RateAnomalyNeedsMinSample`,
      `TestBudget_RateAnomalyStrictThreshold`, `TestBudget_RateAnomalyDisabledByDefault`.)*

## Resolution

Shipped `guard.Budget.RateAnomalous()` (and `ActionRate()`): a run whose
promoted-or-demoted/judged fraction exceeds `MaxActionRate` over at least `MinSample`
judged records is flagged — assessed post-loop on the full sample and folded into the
existing `errAnomalyHalt` path. Because it is gated on the *rate*, not a raw count, it
fires under a throughput cap where the count-anomaly is moot. Default **off** mirrors the
cap's rollout (#0084). See ADR-0022 §Update.
