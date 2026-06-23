---
id: 0092
title: TestStructuredLoggingEmitsSafeFields flakes under the full -race suite (logging-concurrency nondeterminism)
status: open
severity: low
group: 
depends_on: []
forgejo: 381
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

`TestStructuredLoggingEmitsSafeFields` (`internal/server`) **flakes** under the full
`make ci` race suite: it failed once during `go test -race ./...` but passes
consistently in isolation and when its own package is run alone under `-race`. A flaky
gate test erodes trust in CI (the boy-who-cried-wolf failure mode that #0084/ADR-0022
already fought on the anomaly-halt signal) and can mask a real regression on a re-run.

## Repro
1. `make ci` (i.e. `go test -race` across all packages) — observed one failure of
   `--- FAIL: TestStructuredLoggingEmitsSafeFields`.
2. `go test ./internal/server/ -run TestStructuredLoggingEmitsSafeFields` — passes.
3. `go test -race ./internal/server/` (whole package alone) — passes.

Expected: the test is deterministic — green on every run, in isolation and in the full
`-race` suite.
Actual: intermittently red only under the full concurrent `-race` run, so the failure
is timing/ordering dependent, not a product bug.

## Evidence

Surfaced 2026-06-23 while landing #0069 PR1 (PR #380). PR #380 touched only the
`search` handler's session extraction + the telemetry decision field — nothing in the
logging path — and the failure did not reproduce on the same tree in isolation or for
the whole `internal/server` package under `-race`, so this is a **pre-existing**
logging-concurrency flake, not a regression introduced by #380.

## Notes

**Likely cause:** nondeterminism in how the structured-logging test captures/asserts
log fields concurrently with other server tests — e.g. a shared/global logger or output
buffer, a default `slog` handler writing to a process-wide sink, or an assertion that
races a background goroutine's emit. The `-race` scheduler perturbs timing enough to
expose it.

**Fix direction:** make the test self-contained and deterministic — give it its own
`slog.Logger` over a per-test `bytes.Buffer` (no process-global handler), drain/flush any
async emitters before asserting, and avoid asserting on output that another test or a
background goroutine can interleave into. Confirm with `go test -race -count=50 -run
TestStructuredLoggingEmitsSafeFields ./internal/server/` (and a few full-suite `-race`
runs) before closing. Filed **ungrouped**: a self-contained test-determinism fix (no open
epic is a clean fit — #0009 is security-screening, #0008 is feature phases).
