---
id: 0074
title: Ingest the 85 Sonnet advisory labels as advisory gold cases (uses #0063 routing)
status: closed
severity: medium
group: 0015
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
Split from #0063 part (b). #0063 shipped the advisory-prompt **routing** (a gold
case whose record `IsAdvisoryClass` renders via `BuildAdvisoryPrompt`, exempt from
the repro requirement) + one demonstrative case (`ADV1`). This issue is the **data**
half: ingest the 85 Sonnet advisory verdicts from `runs/sonnet-advisory-audit.json`
(66 approve / 19 reject) as advisory gold cases so the off-pool judges (gpt-oss /
gemini) can be calibrated on the real importer-bug set after the Sonnet window closes.

## Approach
For each audit entry (id / decision / failed-checks), load the advisory record from
the corpus and emit a gold case (mode from decision, want_failing_checks from the
audit). Likely needs a small ingest helper (extend `goldadd`) since hand-authoring 85
inline records is impractical; consider a separate `advisory-gold.yaml` so the
prose gold set stays readable.

## Acceptance
- [x] The 85 Sonnet labels are loadable as advisory gold cases (id/decision/checks).
- [x] `judge-eval` scores them via the advisory prompt (the #0063 routing).
- [x] The gold set stays internally consistent (LoadGold passes) and CI-guarded.

## Status

Shipped. The 85 verdicts (66 approve / 19 reject) live in a generated, embedded
`internal/judgeeval/advisory-gold.yaml`, kept separate from the hand-written prose
`gold.yaml` so the latter stays readable; `LoadGold` merges both and validates the
combined set. A bulk generator — `twiceshy gold-add -advisory-audit
runs/sonnet-advisory-audit.json` (`BuildAdvisoryGold`, pure core) — resolves each
audited record from the corpus and emits it as an advisory-class gold case (no repro):
approve → `mode approve`; reject → its `failed_checks` (mode = first). The 2 rejects the
audit left with an empty `failed_checks` array have their checks recovered from the
reason prose ("… fails meaning and poison checks") rather than guessed. `GoldCaseStanza`
now exempts advisory-class records from the repro requirement (mirroring `LoadGold`,
#0063). Guards: `TestLoadGold_IncludesSonnetAdvisorySet` (66/19, all advisory-routed,
no repro), the combined-set consistency/routing tests, `BuildAdvisoryGold` unit tests,
and a golden regeneration test asserting the committed embed equals the generator's
output (catches drift). Enables #0005/#0058 to measure the cheap off-pool judges on the
real importer-bug set.

## Notes
Depends on #0063 (routing, merged). Pairs with #0062 (the `fixed:null` render fix can
be regression-guarded by an advisory gold case). Enables #0005/#0058 to measure the
cheap judges on real importer bugs. Data: `runs/sonnet-advisory-audit.json`.
