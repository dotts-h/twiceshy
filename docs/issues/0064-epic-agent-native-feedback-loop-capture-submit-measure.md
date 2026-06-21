---
id: 0064
title: "Epic: Agent-native feedback loop — capture, submit, measure"
status: open
severity: high
group:
depends_on: []
forgejo: 250
links:
  adr: docs/adr/ADR-0013-closed-loop-autonomous-validation.md
  prs: []
  issues: [0005, 0065, 0066, 0067, 0068]
  regression:
assets: []
---

## Summary
twiceshy's read path and the agent **write/feedback substrate** are already
built and secure: `record_experience`, `report_outcome`, `confirm_helpful` —
all quarantined, judge-gated, content-screened, authenticated, and wrapped in
the `DATA-not-instructions` envelope. The gap is **not** the substrate. It is
that the substrate is **under-triggered, under-surfaced, and unmeasured**:

- **(a) No trigger to write.** Pull is self-targeting (being stuck *is* the
  trigger); the write path has no intrinsic trigger, so agents solve a trap and
  never submit it. Observed directly: a real session found traps and never once
  considered submitting them.
- **(b) No surface for half-formed input.** `record_experience` demands a
  complete lesson; `report_outcome` is record-specific. "I hit X, no fix yet"
  and "twiceshy itself misbehaved" have nowhere to go.
- **(c) The measurement loop is open.** Usage counters do not influence ranking
  (by design, ADR-0013 §4) and only *aggregate* counts are logged — so
  "did the injected card actually help?" is **unmeasurable on real traffic**.

This epic reframes the eval direction (issue #0005) from human-labeled sets to
**agent self-report + off-pool judge analysis**, and closes the loop by reusing
existing pieces (the judge seam, the report spool intake, the content screen,
the transport envelope) rather than adding new architecture.

## Children (suggested build order; soft edges noted, no hard cycles)
1. **#0067 per-query gate-decision telemetry** — foundational substrate; makes
   precision/recall measurable on real traffic and feeds the retro helpfulness
   signal. Unblocked.
2. **#0065 session-retro capture hook** — the headline fix for "agents never
   submit". Reuses #0067's decision log to know which cards were used vs ignored.
3. **#0066 agent issue-submission tool** — self-contained surface for
   half-formed problems / twiceshy-self bugs. Unblocked, parallelizable.
4. **#0068 global-IDF specificity (ADR proposal)** — independent root-cause
   quality fix for the push gate (exp-0622). Unblocked, parallelizable.

## Acceptance (tick when the epic closes)
- [ ] #0065, #0066, #0067, #0068 all closed.
- [ ] An agent session that solves a novel trap results in a quarantined draft
      **without the agent explicitly calling `record_experience`** (#0065).
- [ ] Precision/recall reported on a sample of **real** agent traffic, not only
      the synthetic negative/positive sets (#0067 → #0005).
- [ ] An agent can file a half-formed issue that lands in `docs/issues/` +
      Forgejo, quarantined and deduped (#0066).
- [ ] An ADR records the global-IDF decision; calibration explicitly deferred
      (#0068).

## Notes
- **Security is inherited, not rebuilt.** Every new path (retro endpoint,
  `report_issue`) goes through the existing auth + content-screen + quarantine +
  `DATA-not-instructions` envelope. The retro analyzer must treat session text
  as DATA (an LLM analyzer is itself prompt-injectable). LAN-only; no secret
  exfiltration.
- **Dogfooding is the data source.** Real value comes from running our *other*
  apps against twiceshy and harvesting the resulting traffic + retros — this
  epic is what makes that harvest possible and measurable.
- Relates to exp-0622 (push df-gate trap) and the deferred ADR-0006
  (score-banding). Supersede, never relitigate the locked ADRs silently.
