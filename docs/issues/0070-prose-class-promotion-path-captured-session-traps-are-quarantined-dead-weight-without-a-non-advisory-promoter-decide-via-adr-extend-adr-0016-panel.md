---
id: 0070
title: Prose-class promotion path — captured session traps are quarantined dead-weight without a non-advisory promoter (decide via ADR, extend ADR-0016 panel)
status: closed
severity: high
group: 0064
depends_on: []
forgejo: 298
links:
  adr: docs/adr/ADR-0020-prose-class-panel-promotion.md
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
- [x] An ADR records the prose-promotion decision (supersede/extend ADR-0013 §5 and
      ADR-0016, or commit to a review process), with the poison-risk tradeoff explicit.
      → [ADR-0020](../adr/ADR-0020-prose-class-panel-promotion.md): **Option A** (horia
      directed "A — autonomous panel, live"), superseding ADR-0013 §5 for the prose class.
- [x] If A: a prose-class promoter exists — gated, fail-safe, guardrails ≥ the advisory
      panel; a captured prose trap can reach `validated` autonomously.
- [~] If B: not chosen.
- [x] Captured session traps demonstrably become servable (pull/push) via the chosen path
      — proven at the mechanism level (a prose record promotes to `validated`, which the
      pull/push paths serve). The live demonstration rides the deploy glue below.

## Status

**Option A shipped.** [ADR-0020](../adr/ADR-0020-prose-class-panel-promotion.md) records the
decision (the poison-risk reversal of ADR-0013 §5, made explicit, answered by guardrails
*stronger* than the advisory panel). The promoter:

- **`record.IsProseClass`** = `!IsAdvisoryClass && !HasPositiveRepro` — the residue that
  routes to neither the §1 proof path nor the ADR-0016 advisory panel.
- **`promote.promoteProse`** (routed before the proof-eligibility skip): a quarantined,
  screen-clean, non-disputed prose record is judged by a **cross-family panel** — gpt-oss
  (off-pool local) + **agy** (operator-designated, privacy-acceptable; the **gemini free
  tier is excluded** for prose, ADR-0016 §5; the §6 local denylist stays fully enforced —
  no denylisted model judges). Unanimous approve → `validated` with the panel audit in
  `provenance.promotion`. Fail-safe in every direction (nil panel / any member error /
  any dissent → quarantined).
- **Stronger-than-advisory guardrails:** a **mandatory** clean content-screen
  (`EligibleProse` holds a security-flagged record), a **poison-foregrounded**,
  reject-on-uncertainty prompt (`ProsePanelSystemV1` / `BuildProsePanelPrompt`), the
  born-stale (`valid.until`) gate, and the ADR-0013 §2 veto window.
- Wired in `cmd/twiceshy` from `TWICESHY_PROSE_PANEL_JUDGE_URL/MODEL` (the agy seat),
  each member majority-wrapped (§F1). Tested with stub panels: promotes / one-dissent-holds
  / member-error-fail-safe / no-panel-skip / security-flagged-held / Prose-flag-routes.

**Deferred (deploy glue — "the easy part," per this issue's Notes):** the agy judge **shim**
(CLI→HTTP, like the gemini/sonnet shims), the **longer prose veto-cooldown** config on the
held-PR timer (ADR-0020 §2d — an ops knob, like ADR-0016 §4), a **prose gold set**
(positive + adversarial-poison cases) to measure the panel in `judge-eval` (ADR-0020
backstop; mirrors #0074), and enabling the #0065 SessionEnd capture hook. None are required
by the promoter mechanism this delivers.

## Notes
Found while preparing to enable retro capture live (#0065): the deployment is downstream
of *this* decision — enabling capture before prose-promotion just produces unreviewed
drafts. Relates to ADR-0013 §5, ADR-0016, ADR-0018; epic #0064. The retro **deploy glue**
(a `retro-intake` wrapper/systemd unit + verifying the off-pool shim can serve the
analyzer prompt + registering the SessionEnd hook) is the easy part, tracked separately
when capture is enabled.
