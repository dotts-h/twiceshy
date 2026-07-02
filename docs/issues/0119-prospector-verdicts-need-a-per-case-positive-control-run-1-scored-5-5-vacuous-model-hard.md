---
id: 0119
title: Prospector verdicts need a per-case positive control — run 1 scored 5/5 vacuous model-hard
status: closed
severity: high
group: 0112
depends_on: []
forgejo: 498
links:
  adr: docs/adr/ADR-0029-model-hard-trap-prospector.md
  prs: [500]
  issues: [0112, 0113, 0005]
  regression:
assets: []
---

## Summary
First live prospector run (2026-07-01, qwen2.5-coder:14b as drafter+runner, -max 5):
`scanned 17, eligible 6, drafted 5, model-hard 5, on-also-fails 5`. A 100%
both-arms-fail rate is the instrument-invalidity tell, and case inspection confirms
it — the verdicts are vacuous, failing on infrastructure, not on the trap:
- The drafter chose `verify: gobuild` for ALL cases, including one whose task asks
  for a **GitHub Actions workflow** (exp-0005) — the model correctly answers with
  YAML, which `go build` rejects. Any non-Go-program answer scores "trap hit".
- `gobuild` compiles a bare main.go with no module fetch, but the drafter emitted
  third-party deps (`github.com/mattn/go-sqlite3@v1`, cgo!) — both arms fail
  `go build` on missing modules regardless of trap avoidance.
- Models answer "write a function" prompts with a function, not a `package main`
  program — compile-shape failure, again independent of the trap.

This is the same vacuous-verdict class the #0005 slice-2 control caught (npm HOME
bug → everything "avoided"), in the opposite direction (everything "model-hard").
The `on-also-fails` visibility requirement (ADR-0029 / #0114) did its job: it made
the invalidity impossible to miss. exp-3600's lesson applies: the validity
assumption must be an executable check per case, not a hope.

## Fix (in order of leverage)
1. **Per-case positive control (the core fix):** a case's verdict is admissible only
   if a KNOWN-GOOD solution passes the same verify job. Cheapest v1: the drafter must
   also emit a `control` snippet (a correct, trap-avoiding answer); the prospector
   runs it through the verifier first — control fails ⇒ case voided, counted
   `skipped(control)`, never a verdict. (Mirrors TestLive_VerifierDiscriminates,
   internal/agenteval/live_integration_test.go:82, per case.)
2. **Verifiable-shape constraints in the drafter prompt:** the task must demand a
   single self-contained compilable file (`package main` for Go; a .ts/.tsx module
   for tsc), stdlib-only for gobuild until module fetch exists; forbid
   workflow/config/YAML-shaped tasks.
3. **gobuild deps or narrower eligibility:** either add a networked prepare phase for
   Go modules (like the tsc class's npm install) or have the drafter mark
   third-party-dep Go records unsupported.

## Acceptance
- Re-run of the live prospect over the same 17 records: no case reaches a verdict
  whose positive control fails; skips are counted by reason (`control`).
- A hermetic test: stub verifier failing the control ⇒ case voided, no ModelHard entry.
- The react19-useref gold case, run through the prospector path, still yields a
  verdict (control passes).

## Notes
Run-1 report preserved at the session scratchpad (prospect-run1.json); no gold was
emitted from the run (-gold-out not set). Also noted: using the same model as
drafter and runner biases task difficulty — acceptable for smoke, worth a distinct
drafter model once verdicts are valid.

## Close-out (2026-07-02, PR #500)
Fixes 1+2 landed, built TDD-first via helm-tdd (run 4b0f7bc1; 2 slices, mutation
1.0 each): drafter contract requires a non-empty `control` (rejected in
parseDraftedTask like an empty prompt), `Prospect` verifies the control through
the same `Verifier.Avoided` before any arm — a failing control voids the case as
`Skipped["control"]` (no verdict, not counted Drafted) — and the drafter system
prompt now demands verifiable shapes (single stdlib-only `package main` for
gobuild / single .ts(x) for tsc; workflow/config/YAML answers forbidden).
Hermetic acceptance covered (control-fail voids, verifier-error aborts,
control-pass leaves the verdict path unchanged). Deferred to the next live
session: the 17-record live re-run and the react19-useref gold-case check
(acceptance bullets 1 and 3); fix 3 (gobuild module fetch or narrower
eligibility) intentionally not built — the shape constraints make the drafter
mark third-party-dep Go tasks stdlib-only or unsupported.
