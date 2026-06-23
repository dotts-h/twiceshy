---
id: 0085
title: Promotion-rate anomaly that survives a throughput cap (successor to the count-anomaly retired in capped mode)
status: open
severity: medium
group: 0034
depends_on: [0084]
forgejo:
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

## Acceptance (draft)
- [ ] A run with an abnormally high approve-rate over a minimum sample halts/alerts
      even when `-max-promotions` is set.
- [ ] A normal mixed run (most records held) does not trip it.
- [ ] Baseline + minimum-sample are flag/env tunable; defaults documented in ADR-0022.
