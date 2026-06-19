---
id: 0038
title: "Route guardrail trips to a channel (ntfy notify seam)"
status: open
severity: high
group: 0034
depends_on: [0037]
forgejo: 128
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The anomaly/emergency-stop/budget-cap signals only print to stdout — invisible under cron. Add a small `internal/notify` seam (env-gated `TWICESHY_ALERT_URL` → ntfy, which the brain already runs) and fire on each guardrail trip at slog Warn.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §B3.

## Touches

new `internal/notify`; `cmd/twiceshy/main.go` guardrail sites; optionally `internal/guard`.

## Acceptance

- [ ] An anomalous run posts to the ntfy topic.
- [ ] Unset `TWICESHY_ALERT_URL` is a silent no-op.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
