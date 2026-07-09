---
id: 0144
title: Drafted-task quality: 56% control-fail loss, task-record relevance mismatches produce noise model-hard verdicts, and the report lacks per-record skip reasons
status: open
severity: high
group: 0112
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: [0112, 0140, 0119, 0061]
  regression:
assets: []
---

## Summary
The 0140 live run exposed the drafter as the loop's weakest link, three ways:
(1) **56% control-fail loss** — 126 of 225 eligible records skip because the
drafter's own "correct" control answer fails its drafted task's verify (incl.
exp-2868/react19-useref, whose hand-built task works); (2) **task-record
relevance mismatch** — e.g. exp-2861 (an issue-workflow lesson) drafted as
"multiply two numbers", so its model-hard verdict says nothing about the
record; a relevance guard (shingle/judge between task prompt and record
symptom) should void such drafts like the leak guard does; (3) **the report
aggregates skips** — answering "why was record X skipped?" required a
one-record re-run; per-record skip reasons in the report JSON would make
every future audit cheap.

## Repro
1. `twiceshy prospect` over the validated corpus; inspect the report.
Expected: control-fail is a tail, mismatched drafts are voided+counted, and
each skipped record carries its reason in the report.
Actual: control-fail is the dominant bucket (56%), mismatched drafts reach
verdicts, and skips are aggregate counts only.

## Evidence
- runs/prospect-20260708-0140-live.json: Skipped{control:126} vs Drafted 17.
- exp-2861 model-hard entry: prompt "Write a function that takes two numbers
  and returns their product" vs card "Do not close issues with failing
  acceptance criteria".
- exp-2868 one-record re-run: skipped(control) - vs the working hand-built
  react19-useref task.

## Acceptance
- Control-fail rate materially reduced (better drafter prompt/shapes) or the
  loss is understood and documented per verify class.
- A task-record relevance guard voids mismatched drafts (counted, like leak).
- Report carries per-record outcome (drafted/skipped+reason/verdict).

## Progress

- [x] **Acceptance 3 — per-record skip reasons (PR #574).** `ProspectReport.SkipReasons`
  (record id → skip category) is populated at every skip site (ineligible / unsupported /
  leak / deps / control) and serialized in the report JSON, so "why was record X skipped?"
  needs no one-record re-run. This is the enabler the other two acceptance bullets lean on.
- [ ] **Acceptance 1 — control-fail (56%).** Deferred: a live drafter-quality investigation.
  The 0140 run characterized it (e.g. exp-2868/react19-useref: the drafter's own control
  answer fails its drafted task), but the root-cause + drafter fix needs iterative live
  prospector runs (now cheaper — the abort bugs #0142/#0143 are fixed, and skip reasons are
  per-record). Best done as a focused live session.
- [ ] **Acceptance 2 — relevance guard.** Deferred with acceptance 1: the leak guard's
  5-word-shingle containment (tuned for near-verbatim leaks) scores ~0 for legitimate
  task-vs-symptom relevance, so a relevance floor needs a different measure (token overlap
  or a judge) calibrated against live drafts — the same live loop as acceptance 1.

## Notes
