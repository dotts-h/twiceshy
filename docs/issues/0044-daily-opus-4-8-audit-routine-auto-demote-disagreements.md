---
id: 0044
title: "Daily Opus-4.8 audit routine (auto-demote disagreements)"
status: closed
severity: high
group: 0034
depends_on: [0043, 0036]
forgejo: 134
links:
  adr: ADR-0013
  prs: [159]
  issues: []
  regression:
assets: []
---

## Summary

The morning review is blind without a second opinion. A scheduled headless Claude Code (Opus 4.8 → Fable 5 when available) session reads the night's run manifest (#0036), re-judges each promotion at full reasoning (seeing more than the 20B: full body, diff, versions), auto-demotes/flags disagreements (via #0048/adapt), and posts an ntfy digest. ADR-0013's named escape hatch for a compromised judge.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §F2.

## Touches

a /schedule routine or systemd timer + a small audit script; reads #0036 JSON; writes via #0048.

## Acceptance

- [x] The morning after a run, a digest lists promotions + the audit's agree/disagree per record.
- [x] Disagreements are demoted or flagged.
- [x] OPERATOR STEP: schedule the routine.
- [x] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
