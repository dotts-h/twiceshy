---
id: 0113
title: Prospector core: record-to-task templating, OFF-arm run, broker verdict, model-hard report
status: closed
severity: high
group: 0112
depends_on: []
forgejo: 492
links:
  adr: docs/adr/ADR-0029-model-hard-trap-prospector.md
  prs: [488]
  issues: [0112, 0005]
  regression:
assets: []
---

## Summary
The core measurement loop per ADR-0029. A `TaskDrafter` seam
(`DraftTask(ctx, rec) (TaskCase, error)`, `ErrTaskUnsupported` for a record no
verify class can handle) with a `ModelTaskDrafter` implementation over an
OpenAI-compatible endpoint (mirroring `ModelRunner`'s edge,
`internal/agenteval/runner.go:105`) emitting STRICT JSON
`{prompt, verify, deps}`. Two v1 verify classes: `"tsc"` (npm deps derived from
the record's `applies_to`, following `BrokerVerifier.tscJob`,
`internal/agenteval/verifier.go:111`) and `"gobuild"`. `TaskCase`
(`internal/agenteval/agenteval.go:18`) gains a `Deps` field so a drafted task
carries its own dependency set instead of the hard-coded per-`VerifyID` strings
`buildJob` (`internal/agenteval/verifier.go:77`) uses today; `buildJob` gains
the two generic, parametrized classes alongside the existing three fixed
`VerifyID`s (`fts5-match`, `react19-useref`, `rn-viewstyle`), which are
unchanged. Leak guard: the drafted prompt is checked for word-shingle overlap
(`internal/similarity.Assess`, `internal/similarity/similarity.go:58`) against
the record's resolution text — overlap at or above threshold
(`Report.Flagged`, `similarity.go:80`) skips the record and is counted, so a
drafted task can never hand the model the trap it's meant to probe.
Eligibility mirrors the push channel (ADR-0028): `status = validated`,
`kind ∈ {trap, fix}`, non-importer `provenance.source.author`. OFF arm is a
single run per record (sampling deferred, ADR-0029 decision 5). New
`twiceshy prospect` command: `-corpus`, `-max`, `-report` (writes
`runs/prospect-<ts>.json`), reading `TWICESHY_AGENTEVAL_URL` /
`TWICESHY_AGENTEVAL_MODEL` / `TWICESHY_AGENTEVAL_KEY` (plus optional drafter
overrides). Hermetic in CI via stubbed drafter/runner/broker; live runs are
env-gated.

## Repro
1. Run `twiceshy prospect -corpus <path> -max N` against the live validated
   corpus with `TWICESHY_AGENTEVAL_URL` unset.
Expected: the command reports the loop is not configured (env-gated, no live
call attempted) and exits cleanly — no partial/fake report is written.
Actual: no such command exists yet; there is no automated way to find which
validated records a base model actually fails.

## Evidence
- `internal/agenteval/runner.go:105` (`ModelRunner.Run`) is the existing
  OpenAI-compatible chat-completions edge the drafter's HTTP shape should
  mirror.
- `internal/agenteval/verifier.go:56` (`BrokerVerifier.Avoided`) and `:77`
  (`buildJob`) are the existing verdict path; `:111` (`tscJob`) is the existing
  tsc scaffold the two new deps-parametrized classes generalize.
- `internal/agenteval/agenteval.go:18` (`TaskCase`) has no `Deps` field today —
  the three existing `VerifyID`s hard-code their npm deps inline in
  `verifier.go`'s `tscJob` calls.
- `internal/similarity/similarity.go:58` (`Assess`) and `:80` (`Flagged`) are
  the existing near-verbatim-overlap check from #0090, reused here as the leak
  guard rather than built new.
- ADR-0028's push-eligibility rule (`kind ∈ {trap, fix}`, non-importer origin)
  is the model this issue mirrors for prospector eligibility.

## Acceptance
- `TaskDrafter` interface + `ModelTaskDrafter` land with a hermetic-stub test
  suite (no live HTTP in CI).
- `TaskCase.Deps` is added and both new verify classes (`tsc`, `gobuild`) are
  wired into `buildJob` without changing the three existing fixed `VerifyID`s'
  behavior.
- The leak guard skips (and counts) any drafted task whose prompt shingle-
  overlaps the record's resolution text at or above the chosen threshold.
- `twiceshy prospect` runs against a stub drafter/runner/broker in CI and
  against the live corpus + local models when `TWICESHY_AGENTEVAL_*` is set;
  the report JSON lists scanned/drafted/skipped(reasons)/model-hard counts.
- Eligibility filtering matches ADR-0028's push rule exactly (validated,
  trap/fix, non-importer author).

## Notes
`ErrTaskUnsupported` is an honesty signal, not a failure: a record with no
matching verify class is skipped and counted, never forced into a bad task
that "cannot fake a delta" claim ADR-0029 relies on. Depended on by #0114
(gold emission), which consumes this issue's OFF-arm failures.
