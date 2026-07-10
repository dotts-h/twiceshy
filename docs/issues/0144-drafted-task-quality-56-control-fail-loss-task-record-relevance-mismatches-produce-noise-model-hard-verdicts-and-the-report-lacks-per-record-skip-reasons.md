---
id: 0144
title: Drafted-task quality: 56% control-fail loss, task-record relevance mismatches produce noise model-hard verdicts, and the report lacks per-record skip reasons
status: closed
severity: high
group: 0112
depends_on: []
forgejo: 587
links:
  adr:
  prs: [574, 575]
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

## Resolution (2026-07-09)

All three acceptance bullets addressed across PRs #574 (part 3) and #575 (parts 1+2),
the latter driven by a live prospector investigation.

- [x] **Acceptance 3 — per-record skip reasons (PR #574).** `ProspectReport.SkipReasons`
  (record id → skip category) is populated at every skip site and serialized in the report
  JSON, so "why was record X skipped?" needs no one-record re-run.

- [x] **Acceptance 1 — control-fail root cause UNDERSTOOD & DOCUMENTED (the "or" clause).**
  A live probe (`internal/agenteval/probe_controlfail_test.go`, PROBE_CONTROLFAIL=1,
  qwen2.5-coder:14b + docker/runsc) drafted real records and ran their controls through the
  broker. The dominant cause is **not** the trap biting — it's that the eligibility filter
  (validated ∧ kind∈{trap,fix} ∧ non-importer) admits many records that are **not
  code-reproducible traps**: workflow lessons (exp-2861 "do not close issues with failing
  CI"), sandbox-infra lessons (exp-4231 "offline build needs a pre-fetched module"), and
  ops/deploy lessons (exp-2840 "scheduled job dies on binary↔script version skew"). The
  drafter faithfully tries and **fabricates a generic, unrelated task** whose own control
  then fails `tsc`/`gobuild` for reasons unrelated to any trap:
    - exp-2861 → drafted "concatenate two strings" (zero relation to the record).
    - exp-4231 → drafted "print a random integer" (zero relation).
    - exp-2868 (react19-useref) → drafted a `useRef<number>()` attached to a `<div>`, failing
      TS2322 (an incidental type error), never even exercising the trap's TS2554.
  So control-fail is dominated by **drafter fabrication on non-code-reproducible records**,
  concentrated in the tsc/gobuild classes.

- [x] **Acceptance 2 — relevance guard (PR #575).** The guard voids a drafted task whose
  prompt shares NONE of the record's distinctive terms (title + symptom + error_signatures +
  applies_to package, tokens ≥4 chars minus a generic-coding stopword set) — catching the
  fabrications above before they burn a control-verify or emit a noise model-hard verdict.
  It under-voids by design (one shared term keeps a draft), so a relevant-but-hard draft is
  never silenced: verified live that exp-2861/exp-4231 void while exp-2868 (shares
  "useref"/"react") is kept (`TestProbe_RelevanceGuardCalibration`, `Skipped["irrelevant"]`).

Follow-up (not blocking): tightening `prospectEligible` to exclude non-code-reproducible
kinds up front would cut the fabrication rate at the source; the relevance guard already
neutralizes the noise downstream.

## Notes
