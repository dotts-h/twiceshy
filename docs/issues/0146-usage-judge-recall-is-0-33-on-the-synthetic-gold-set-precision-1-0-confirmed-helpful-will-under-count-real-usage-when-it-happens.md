---
id: 0146
title: Usage judge recall is 0.33 on the synthetic gold set (precision 1.0) — confirmed_helpful will under-count real usage when it happens
status: closed
severity: medium
group: 0064
depends_on: []
forgejo:
links:
  adr:
  prs: [573]
  issues: [0064, 0069]
  regression:
assets: []
---

## Summary
The #0069 acceptance-3 measurement (2026-07-08): on the synthetic gold set the
ModelUsageJudge (gpt-oss:20b via the :8729 shim) scores precision 1.0 / recall
0.33 - it judged both genuinely-used cards in the "both-used" case as ignored.
On the first real-traffic sample (8 sessions, 18 served pairs, gold all-
ignored) it emitted zero false "used" verdicts. Conservative is the right
failure direction (no fake confirmations), but recall 0.33 means the flywheel
signal will under-count real usage when it starts happening. Improve the judge
prompt/shim (e.g. explicit "a card is used if its lesson is applied, even
uncited"), re-measure on the synthetic set, and keep FP=0 on the real set.

## Repro
1. `TWICESHY_RETRO_MODEL=gpt-oss:20b TWICESHY_RETRO_URL=http://127.0.0.1:8729 twiceshy eval -usage -json`
Expected: recall near 1.0 on the deliberately unambiguous synthetic cases.
Actual: {TP:1, FN:2, recall:0.33} - "both-used: exp-0002/exp-0006 judge=ignored gold=used".

## Evidence
- Synthetic: precision 1.0 / recall 0.33 (both-used case: 2 FN).
- Real sample (usage-cases-real-20260708.json, PRIVATE, outside the repo):
  8 cases / 18 pairs, FP 0.

## Acceptance
- Synthetic recall materially improved without dropping precision below 1.0
  on the real all-negative sample (re-run both evals; record numbers here).

## Resolution (2026-07-09)

Root cause was NOT prompt wording — it was the **shim contract** (the #0099 class).
`ModelUsageJudge` sent an empty `system`, so the shared retro-analyzer shim (:8729)
injected its OWN default system — the trap-EXTRACTION contract ("respond with
`{\"candidates\":[...]}`") — and gpt-oss dutifully returned candidates, never verdicts.
`wireVerdicts` decoded zero verdicts → every served card scored FN → recall collapsed
(measured 0.00 live at repro time, worse than the filed 0.33).

Fix (`internal/retro/model_usage.go`): `NewModelUsageJudge` now defaults `system` to a
verdicts-contract `defaultUsageSystem` when none is configured — it also states the recall
rule ("a card is used if its lesson is APPLIED, even if the id is not cited"). Verified live
against the :8729 shim (gpt-oss:20b):

- Synthetic gold: precision 1.0, **recall 0.00 → 1.00** (TP 3, FP 0, FN 0).
- Real all-negative sample (usage-cases-real-20260708.json, 8 cases / 18 served pairs):
  **FP 0 preserved** (TP 0, FP 0, FN 0) — no false "used" introduced.

Unit guard: `TestModelUsageJudge_ParsesVerdictsAndFramesTranscript` asserts the default
system carries the verdicts contract; `TestModelUsageJudge_ExplicitSystemOverridesDefault`
keeps an explicit `cfg.System` authoritative.

## Notes
