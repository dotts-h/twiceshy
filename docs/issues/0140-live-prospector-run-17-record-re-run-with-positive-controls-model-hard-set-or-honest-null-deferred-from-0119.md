---
id: 0140
title: Live prospector run — 17-record re-run with positive controls; model-hard set or honest null (deferred from 0119)
status: open
severity: high
group: 0112
depends_on: []
forgejo:
links:
  adr: docs/adr/ADR-0029-model-hard-trap-prospector.md
  prs: []
  issues: [0112, 0119, 0113, 0114, 0005]
  regression:
assets: []
---

## Summary
Run the prospector loop live against the validated corpus with the #0119
positive-control fixes in place, and land the result. #0119's close-out (PR
#500) explicitly deferred its two live acceptance bullets: the 17-record live
re-run, and the react19-useref gold-case sanity check. Until that run happens,
epic #0112's own acceptance ("at least one genuinely model-hard record is
found, or an honest null is reported") is unmet — run 1's 5/5 model-hard score
was vacuous (no per-case control), so the project still has zero trustworthy
model-hard verdicts.

## Repro
1. `TWICESHY_AGENTEVAL_URL`/`_MODEL`/`_KEY` set → `twiceshy prospect -corpus
   <validated corpus> -max 17 -report runs/`
Expected: a report where every counted verdict passed its per-case positive
control; model-hard records listed with controls green, or an honest null.
Actual: no post-#0119 live run exists; the only live report predates the
control fix and its 5/5 model-hard result is known-vacuous.

## Evidence
- docs/issues/0119 close-out: "Deferred to the next live session: the
  17-record live re-run and the react19-useref gold-case check (acceptance
  bullets 1 and 3)".
- Epic 0112 acceptance box 3 requires a measured model-hard find or an honest
  null — neither exists yet on a control-guarded run.

## Acceptance
- A live `twiceshy prospect` run over the eligible validated records completes
  with per-case controls enforced; the report's scanned / drafted / skipped
  (reasons) / model-hard counts are recorded in this issue.
- The react19-useref record's known outcome (base model avoids unaided,
  2026-07-01) is reproduced or the discrepancy is investigated — the gold-case
  sanity check from #0119 acceptance bullet 3.
- Any model-hard finds flow to #0005 gold emission per #0114's machinery; a
  null result closes this issue honestly (empty is an answer).
- Epic #0112's acceptance boxes are reconciled against the run's outcome.

## Notes
This is a live, env-gated measurement session (local models via
`TWICESHY_AGENTEVAL_*`), not new engine code — any code changes it forces
(e.g. drafter shape rejections observed live) get filed separately.
