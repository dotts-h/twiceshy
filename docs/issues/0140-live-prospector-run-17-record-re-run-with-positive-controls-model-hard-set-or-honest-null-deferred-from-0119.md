---
id: 0140
title: Live prospector run — 17-record re-run with positive controls; model-hard set or honest null (deferred from 0119)
status: closed
severity: high
group: 0112
depends_on: []
forgejo: 583
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

## Close-out (2026-07-08)

Run complete (attempt 8; report `runs/prospect-20260708-0140-live.json`,
qwen2.5-coder:14b drafter+runner, controls enforced): **scanned 4004,
eligible 225, drafted 17 (the -max cap), skipped: control 126 / unsupported
73 / deps 9 / ineligible 3779. OFF-avoided 12. Model-hard 5 — and 1 of them
(exp-3952) FLIPPED ON-arm: the first measured positive card delta** (epic
0112 acceptance box 3 met with a real find, not a null). Gold set grew by 5
via #0114's merge; the empty-set test evolved into a shape guard.

react19-useref sanity check (acceptance bullet 2): exp-2868 skips as
control-fail in the drafted path — the drafter's own correct answer does not
pass its drafted task, so the case voids honestly; the 2026-07-01 hand-built
fixed-VerifyID result stands as the authority for that record. Investigated,
not reproduced-by-draft: recorded as a #0144 data point.

Getting here forced four engine fixes, each from a live abort: #0142 npm
deterministic-resolution skip family (E404/ETARGET/ERESOLVE/ENOENT — 9
deps-skips in the final run would each have been a fatal abort), #0143
transport retry on the model edges, plus the systemd-unit run harness
(background-timeout immunity). Honest caveats → #0144: 56% control-fail loss,
task-record relevance mismatches make some model-hard verdicts noise (e.g.
exp-2861), and the report lacks per-record skip reasons.
This is a live, env-gated measurement session (local models via
`TWICESHY_AGENTEVAL_*`), not new engine code — any code changes it forces
(e.g. drafter shape rejections observed live) get filed separately.
