---
id: 0063
title: judgeeval lacks advisory-prompt routing — can't measure cheap judges on the Sonnet advisory gold-set
status: open
severity: medium
group: 0015
depends_on: []
forgejo: 195
links:
  adr: ADR-0016
  prs: []
  issues: [0005, 0058, 0061, 0062]
  regression:
assets: []
---

## Summary

`internal/judgeeval` judges every gold case with the prose `BuildPrompt`; there is
no routing to `BuildAdvisoryPrompt` for advisory-class records. So the 85 Sonnet
advisory verdicts from the 2026-06-20 audit (`runs/sonnet-advisory-audit.json`)
can't yet be used as a judge-eval — scoring an advisory record under the prose
rubric measures the wrong thing. This blocks turning the Sonnet gold-set into a
standing calibration for the off-pool judges (gpt-oss/gemini) after the Sonnet
window closes.

## Repro
1. Add an advisory-class gold case (vuln id, no repro) to `gold.yaml` and run the
   judge-eval.

Expected: the harness renders it via `BuildAdvisoryPrompt` (the prompt the
production advisory panel actually uses) and scores the advisory verdict.

Actual: it renders via the prose `BuildPrompt`; the advisory checks
(meaning/scope/license/poison over a no-repro record) are never exercised, so the
score doesn't reflect production advisory judging.

## Evidence

`grep -n 'Advisory' internal/judgeeval/*.go` → no matches (no advisory routing).
The Sonnet audit produced 66 approve / 19 reject labels across the 85 quarantined
advisories (`runs/sonnet-advisory-audit.json`) ready to ingest as advisory gold
cases once routing exists.

## Notes

Fix: (a) route advisory-class records (`record.IsAdvisoryClass`) to
`BuildAdvisoryPrompt` in the eval; (b) ingest the Sonnet labels as advisory gold
cases (id/decision/failed-checks per `runs/sonnet-advisory-audit.json`). Pairs with
#0062 (so the `fixed:null` render fix can be regression-guarded by an advisory gold
case). Enables #0005/#0058 to measure the cheap judges on real importer bugs.
Related: [[ADR-0016]], #0061.
