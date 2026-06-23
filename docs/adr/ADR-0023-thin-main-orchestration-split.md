# ADR-0023: Lift the batch-orchestration core out of `cmd/twiceshy/main.go` into `internal/`

- **Status:** Proposed (2026-06-23) — surfaced by the post-#0086 code-quality /
  architecture audit; claude proposed and authored. Awaiting **horia**'s decision
  before any code moves (it changes a package boundary and relocates ~21 tests).
- **Related:** [ADR-0005](ADR-0005-stable-seams.md) (stable seams — this reuses the
  existing `recordPromoter` / `counterRunner` / `pipelineRunner` / `persist` seams);
  [ADR-0008](ADR-0008-record-persistence-is-a-cli-concern.md) /
  [ADR-0019](ADR-0019-write-path-is-the-autonomous-validation-loop.md) (the write-path
  security kernel — explicitly **not** superseded here); [ADR-0022](ADR-0022-promote-throughput-and-hold-cooldown.md)
  (the #0084 cap/anomaly/cooldown ordering that lives in the code to be moved);
  [CONVENTIONS.md](../CONVENTIONS.md) ("thin `main`; logic lives in `internal/`").

## Context

`cmd/twiceshy/main.go` is **2,693 lines / ~73 functions** — the next-largest file in
the repo is 767. It imports all 20 `internal/` packages and strands load-bearing,
independently-tested business logic in `package main`:

- The corpus-walk drivers `promoteCorpus` / `adaptCorpus` / `draftCorpus`, which carry
  the security- and correctness-sensitive invariants: the #0084 throughput-cap-placed-
  **before** `MaxActions` ordering and the anomaly-halt-**before**-write contract
  (`errAnomalyHalt`, exit 3), and the journal skip/record/abort/complete sequencing.
- `runPromote`'s ~115 lines of judge/panel/staleness-gate assembly — the binary's most
  security-sensitive wiring (ADR-0013 §6 fail-safe panel; ADR-0016/0020 panels).
- The thin `corpusJournal` wrapper, the `*Stats` types, and `maxRecordNum`.

Two costs follow. (1) None of this can be **imported or reused** — a future scheduler,
server-driven run, or a focused integration harness has to shell out to the binary.
(2) `runPromote` and `runAdapt` share a near-identical post-walk **epilogue**
(`guardrailsFrom`, run id/logger, `notify.New`, effect short-circuit, `surfaceJudgeStats`,
the `errAnomalyHalt` branch, `-json RunManifest` emission, conditional `Heartbeat`),
differing only in stat field names and the stage string — so a change to the run
contract must be made in two places in lockstep (the drift the simplicity rule guards
against). The seams these drivers depend on are already interfaces, which is exactly
what makes the lift low-risk *mechanically*; the cost is the test relocation, not the
wiring.

This is the one genuine structural debt the audit found. It is **not** a bug — the code
is correct and well-tested — so it is recorded for a decision, never refactored silently
(per the architecture skill and CONVENTIONS' "thin main").

## Decision (proposed)

1. **Extract a new `internal/run` (working name) package** that owns the three corpus
   drivers + their `*Stats` types + the `corpusJournal` wrapper. It reuses the
   already-exported `promote.Journal` / `RecordAction` / `JournalStop` /
   `LoadJournal` (no re-export work needed — these already live in
   `internal/promote/journal.go`). The drivers keep their existing seams and continue
   to accept `guard.Guardrails`, `*slog.Logger`, `notify.Alerter`, and `io.Writer` as
   injected parameters (no import cycle: `guard` and `notify` do not import `promote`).
   `main.go` becomes a thin caller wiring the broker-backed runner, guardrails, alerter,
   and out writer.
2. **Unify the `runPromote`/`runAdapt` epilogue** behind a small `runStage(...)` loop-
   driver (injected adjudicator + a stage-specific manifest/summary callback) so the run
   contract has one home.
3. **Extract `buildPromoterOptions(getenv, …) ([]promote.Option, error)`** so the
   security-sensitive `TWICESHY_PANEL_*` env→option assembly is unit-testable in
   isolation rather than only end-to-end.
4. Land each as a **pure relocation**, moving the existing `*_test.go` suites unchanged
   so `make ci` and the 80% coverage floor stay green.

Explicitly **out of scope / not superseded:** the `writeRecord` "persistence is a CLI
concern" security kernel (ADR-0008/0019) is orthogonal and stays in `cmd`. `maxRecordNum`
is a draft id-allocation helper, *not* part of the walk, and does not move with the
drivers.

## Consequences

- **+** `main` becomes the thin dispatcher CONVENTIONS mandates; the loop logic gains a
  home that is importable, reusable, and directly testable; the run contract stops being
  duplicated.
- **+** The judge/panel wiring — the highest-stakes code in the binary — gets a unit-test
  seam.
- **−** Medium risk: ~21 tests across `promote_test.go` / `adapt_test.go` / `draft_test.go`
  (+ parts of `anomaly_test` / `guardrails_test` / `effect_test` / `failsafe_test`) move
  packages. It is **not** a mechanical lift and must be staged, with the `-json` manifest
  bytes and exit codes asserted byte-identical before/after.
- **−** Churn against an actively-running autonomous pipeline (the binary is rebuilt and
  redeployed nightly), so it should land as its own reviewed PR, not bundled with feature
  work.
- A cheaper **non-ADR** interim, if the full lift is deferred: a same-package file split
  (move functions into `report.go` / `doctor.go` / `loop.go` with no boundary change)
  recovers most of the navigability win without moving any test or boundary.
