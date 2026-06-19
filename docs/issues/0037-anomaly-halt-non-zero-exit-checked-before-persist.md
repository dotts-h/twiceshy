---
id: 0037
title: "Anomaly = HALT + non-zero exit, checked before persist"
status: closed
severity: high
group: 0034
depends_on: [0036]
forgejo: 127
links:
  adr: ADR-0013
  prs: [152]
  issues: []
  regression:
assets: []
---

## Summary

Today promotions are persisted BEFORE the anomaly check, the run continues, and exits 0 — a compromised judge approving everything writes bad records to disk and succeeds. Check `Budget.Anomalous()` before persisting further actions, stop the run, set a distinct non-zero exit, and surface it in the run summary + an alert.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §D1.

## Touches

`cmd/twiceshy/main.go` promoteCorpus/adaptCorpus (check-before-persist + propagate); main exit mapping.

## Acceptance

- [x] A forced anomaly stops mid-run with no further writes and a non-zero exit.
- [x] The anomaly appears as a field in the run summary (#0036).
- [x] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
