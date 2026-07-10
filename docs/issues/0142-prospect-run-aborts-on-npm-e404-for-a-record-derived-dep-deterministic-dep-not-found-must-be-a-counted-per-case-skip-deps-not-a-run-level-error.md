---
id: 0142
title: Prospect run aborts on npm E404 for a record-derived dep — deterministic dep-not-found must be a counted per-case skip (deps), not a run-level error
status: closed
severity: high
group: 0112
depends_on: []
forgejo: 585
links:
  adr: docs/adr/ADR-0029-model-hard-trap-prospector.md
  prs: [553, 554, 555, 556]
  issues: [0112, 0140, 0119]
  regression:
assets: []
---

## Summary
The 0140 live run (2026-07-08, attempt 2) aborted at record exp-2776: the tsc
verify class npm-installs the record-derived dep `@cosul/db@2`, the public
registry 404s it, `BrokerVerifier.Avoided` surfaces the prepare failure as an
error, and the prospect loop treats every verifier error as run-fatal - so ONE
record whose package does not exist on npm poisons the whole sweep. A
deterministic dep-not-found is a case-input defect (the record cannot be turned
into a verifiable task), morally identical to ErrTaskUnsupported: it must be
counted and skipped (Skipped["deps"]), while genuine substrate failures (docker
down, network, non-404 npm errors) must KEEP aborting - the 0119 lesson that a
partial result must never be silent stands.

## Repro
1. `twiceshy prospect` over a corpus containing a validated tsc-class record
   whose applies_to package is not on the public npm registry (e.g. exp-2776,
   `@cosul/db@2`).
Expected: the case is counted under Skipped["deps"] and the run continues to
the remaining records; the report lists the skip reason.
Actual: `agenteval: tsc prepare failed (exit 1): npm error code E404 ...`
aborts the entire run (exit 1); no report is written.

## Evidence
- internal/agenteval/verifier.go:77 - prepare failure => error (right for
  substrate failures, wrong for deterministic E404).
- internal/agenteval/prospect.go:126 - any control-verify error is run-fatal.
- 0140 run log: attempt 2 died at exp-2776 with `npm error 404 Not Found -
  GET https://registry.npmjs.org/@cosul%2fdb`.

## Acceptance
- `BrokerVerifier.Avoided` classifies an npm-E404 prepare failure as a typed
  sentinel (e.g. ErrDepsUnavailable) wrapped with the case context; all other
  prepare failures keep their current error shape.
- The prospect loop counts an ErrDepsUnavailable at ANY verify site (control,
  OFF, ON) as Skipped["deps"] and continues; other verifier errors still abort.
- Hermetic tests cover both paths (E404 => skip+continue, non-404 => abort).
- The 0140 live re-run completes past exp-2776.

## Notes
