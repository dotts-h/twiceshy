---
id: 0035
title: "Structured slog logging on the promote/adapt loop"
status: closed
severity: high
group: 0034
depends_on: []
forgejo: 125
links:
  adr: ADR-0013
  prs: [150]
  issues: []
  regression:
assets: []
---

## Summary

The autonomous write path emits ZERO structured logs — every action is human prose on stdout, while the read path has full slog. Add an slog logger emitting one JSON event per record decision (run_id, stage, record_id, outcome, reason, judge_model, judge_decision, reproduced_under, attestation_ran_at, duration_ms) alongside the existing prose. Every outcome must log, incl. `held`.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §B1.

## Touches

`cmd/twiceshy/main.go` (runPromote/runAdapt/promoteCorpus/adaptCorpus); reuse internal/server's slog.NewJSONHandler.

## Acceptance

- [x] A run produces a parseable JSON line per record + one summary line.
- [x] held/ineligible/error outcomes all appear (today adapt `held` emits nothing).
- [x] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
