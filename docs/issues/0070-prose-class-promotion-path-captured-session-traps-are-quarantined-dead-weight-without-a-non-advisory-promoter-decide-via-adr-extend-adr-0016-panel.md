---
id: 0070
title: Prose-class promotion path — captured session traps are quarantined dead-weight without a non-advisory promoter (decide via ADR, extend ADR-0016 panel)
status: open
severity: high
group: 0064
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: [0064, 0065]
  regression:
assets: []
---

## Summary
Retro capture (#0065, [ADR-0018](../adr/ADR-0018-session-retro-capture.md)) lands
session traps as **`quarantined`** records. To help any agent they must become
**`validated`** (pull hides quarantined; push serves validated-only). But a captured
**prose lesson** (a `convention`, a narrative gotcha, a "don't do X") has **no
auto-promotion path**: auto-promotion covers only (1) **execution-provable** records —
repro + judge ([ADR-0013](../adr/ADR-0013-closed-loop-autonomous-validation.md) #0029),
and (2) **advisory-class** OSV/GHSA records — judge panel, no repro
([ADR-0016](../adr/ADR-0016-advisory-class-panel-promotion.md)). Prose is **neither**,
so it stays human-gated (ADR-0013 §5, **not** superseded by ADR-0016, which is
advisory-only). **So #0065's captured traps are dead-weight until this is resolved** —
and ADR-0016's own argument is that a human-gated pile *"in practice is never reviewed."*

## Motivation
This is the **real blocker for the value of epic #0064**: capture (#0065) ships, but
captured prose traps reach no agent without a promotion path. Enabling retro capture
without this just grows an unreviewed quarantined pile — the exact failure ADR-0016
named. (Reproducible captured traps are *not* in scope: they can already ride the
existing drafter→proof→judge pipeline, #0026 `twiceshy draft`. The gap is the
**pure-prose** subset, likely the majority of organic session traps.)

## This needs a decision (ADR first)
ADR-0013 §5 deliberately kept prose human-gated for **higher poison risk** ("no proof
+ high poison risk"); ADR-0016 reversed it for advisories *because they are lower
poison risk than prose*. Promoting prose therefore reverses a conscious call and **must
be an ADR** weighing the options:
- **A — prose-class judge panel** with *stronger* guardrails than the advisory panel
  (larger/unanimous panel, the §2 veto window, content-screen, anomaly caps) —
  supersede §5 for the prose class.
- **B — committed lightweight human review** of retro-draft PRs (no auto-promote;
  accept the manual step, but keep the queue small/triaged so it actually *is* reviewed).
- **C — serve quarantined-with-label** to agents — **rejected shape** (ADR-0013 Option
  B, the corpus-poisoning failure mode; listed only to be explicitly ruled out).

## Acceptance
- [ ] An ADR records the prose-promotion decision (supersede/extend ADR-0013 §5 and
      ADR-0016, or commit to a review process), with the poison-risk tradeoff explicit.
- [ ] If A: a prose-class promoter exists — gated, fail-safe, guardrails ≥ the advisory
      panel; a captured prose trap can reach `validated` autonomously.
- [ ] If B: a defined, low-friction review path for retro-draft PRs that is actually run
      (not a never-reviewed pile).
- [ ] Captured session traps demonstrably become servable (pull/push) via the chosen path.

## Notes
Found while preparing to enable retro capture live (#0065): the deployment is downstream
of *this* decision — enabling capture before prose-promotion just produces unreviewed
drafts. Relates to ADR-0013 §5, ADR-0016, ADR-0018; epic #0064. The retro **deploy glue**
(a `retro-intake` wrapper/systemd unit + verifying the off-pool shim can serve the
analyzer prompt + registering the SessionEnd hook) is the easy part, tracked separately
when capture is enabled.
