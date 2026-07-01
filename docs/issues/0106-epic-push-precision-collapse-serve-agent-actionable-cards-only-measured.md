---
id: 0106
title: 'Epic: Push precision collapse — serve agent-actionable cards only, measured'
status: open
severity: high
group:
depends_on: []
forgejo: 472
links:
  adr: docs/adr/ADR-0028-push-eligibility-and-corroborating-specificity.md
  prs: [479, 480]
  issues: [0005, 0067, 0068, 0069, 0105]
  regression:
assets: []
---

## Summary
The push channel's precision has collapsed: measured over live gate-decision
telemetry (2026-06-24 → 07-01, v0.2.8), **70% of push queries inject cards**
(504/723 across 85 sessions), while lifetime usage counters read **12,304
pushed / 1 confirmed_helpful**. The channel that exists to make twiceshy fire
without agent discretion is instead training every agent to ignore the
`EXPERIENCE DATA` block.

Two compounding causes, both verified against the live index:

1. **The df gate inverted as the corpus grew.** ADR-0015's discriminative-token
   gate (`df ∈ [1, pushMaxDF=3]` over validated records) was calibrated on a
   ~30-record curated corpus and never re-confirmed (ADR-0015's own clause) as
   validated grew to 990. At 990 — ~95% OSV advisories — ordinary dev words
   become "rare": sampled 47 everyday words, **17 were discriminative**
   (`rename`, `button`, `email`, `deploy`, `menu`, `login`…). Reproduced
   specimen: prompt *"need a deep analysis of this application and why it is
   still not working well not helping any llm"* fired on `application` (df=2)
   and `llm` (df=2) and injected a Docker-Compose trap + a selftest convention
   card. Meanwhile genuinely topical tokens (`go`, `sqlite`, `react`) exceed
   df=3 and are excluded — the gate now *prefers accidental rarity over topical
   relevance*. This is exp-0622 / ADR-0017's diagnosis recurring at scale;
   #0068 closed as proposal-only, so no IDF signal exists in the gate.

2. **The corpus feeding push is demand-irrelevant.** ~940/990 validated records
   are importer-origin advisories (self-audit material, not mid-prompt
   material), and the prose-panel promotion path validates generic non-lessons
   (exp-2845 "Use Selftests for Argument Parsing Invariants" — a note that
   *a test was once added*, panel-approved). Every such record becomes push
   fodder.

## Children
- 0107 — push eligibility: only agent-actionable origins/kinds reach push
- 0108 — two-token corroboration for prompt-triggered push (error trigger exempt)
- 0109 — flag-gated raw query text on gate-decision telemetry
- 0110 — promotion judge usefulness dimension
- 0111 — static dev-vocabulary stoplist extension (ADR-0017 cheap proxy)

Related, pre-existing: #0005 (prove-or-kill eval — run with gold traps a base
model fails), #0069/#0105 (served→used join), #0068/ADR-0017 (the principled
IDF endgame these children bridge to).

## Evidence
- Live telemetry pull 2026-07-01: 723 push queries / 504 served (70%); 41 pull
  queries, 61% empty; 138 distinct records ever pushed; top = exp-2870 ×116,
  exp-0005 ×111.
- Live usage table: Σpushed 12,304; Σretrieved 103; Σconfirmed_helpful 1.
- df reproduction + 47-word sample against the live `records_fts` (validated
  join), 2026-07-01.
- ADR-0026's 33h measure (2026-06-24): push ~2,462×, pull 2–5×, feedback 0×.

## Acceptance (epic closes when)
- [ ] Off-topic/meta prompts (incl. the two live specimens) inject 0 cards on
      the deployed server.
- [x] Push serves only agent-actionable records (0107) under the corroboration
      rule (0108), with the live precision/recall guard updated and green.
- [ ] Gate decisions are debuggable (0109 deployed with the flag on).
- [x] Prose/advisory promotion asks the usefulness question (0110).
- [ ] Post-deploy telemetry shows served-rate on prompt-triggered push well
      below the 70% baseline without losing error-pull hits.

## Notes
Diagnosed in the 2026-07-01 deep-analysis session (orchestrator: Claude).
ADR-0028 records the policy; ADR-0017 remains the endgame for specificity.
