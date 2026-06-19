---
id: 0049
title: "True effect-preview dry-run"
status: open
severity: medium
group: 0034
depends_on: []
forgejo: 139
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

`-dry-run` only lists candidates (skips gate+judge) so it can't preview a batch's actual outcome. Add a mode that runs gate+judge, writes nothing, and prints the would-be status delta per record.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §C3.

## Touches

`cmd/twiceshy/main.go` promoteCorpus/adaptCorpus (a no-persist mode); reuse the dry-run flags.

## Acceptance

- [ ] `promote -dry-run -effect` prints `exp-X: quarantined→validated` per record and writes nothing.
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
