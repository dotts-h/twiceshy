---
id: 0033
title: Guardrails for autonomous promotion — anomaly monitoring, emergency stop, budget caps
status: closed
severity: high
group: 0027
depends_on: [0029, 0031]
forgejo:
links:
  adr: ADR-0013
  prs: [86]
  issues: [0032]
  regression:
assets: []
---

## Summary

The safety net ADR-0013 §7 makes part of the decision (surfaced by the diverse-model
review): autonomous promotion does not ship without **(a) anomaly monitoring**,
**(b) an emergency stop**, and **(c) budget caps**. These cover the residual risks
the gate + judge can't — chiefly an **available-but-compromised judge** ("who judges
the judge") and a **`report_outcome` DoS**.

## Touches

A monitoring/metrics surface (promotion + demotion counters, judge verdict rates) +
a global pause flag read by the promotion path (#0029) and the counter-evidence path
(#0032) + a per-window budget on broker/judge invocations triggered by reports
(#0031).

## Acceptance

- [ ] **Anomaly monitoring**: promotion/demotion rate + pattern alerts (e.g. a sudden
  spike in approvals, or a class of cards being demoted) raise a notification; a
  judge that starts approving everything is caught, not discovered in production.
- [ ] **Emergency stop**: a single switch halts all auto-promotion (and auto-demote);
  records pile up `quarantined`/`disputed` (fail-safe), nothing auto-releases while
  engaged.
- [ ] **Budget caps**: a ceiling on broker/judge runs a report can trigger per window;
  exceeding it queues/drops with a logged reason rather than draining the sandbox.
- [ ] Test-first; `make ci` green.

## Notes

Pairs with the §2 veto window: monitoring + the soak window + the outcome-feedback
loop are the layered cover for a compromised judge (ADR-0013 Threats). Independent of
the judge's own logic — these watch the *system*, not the verdict.
