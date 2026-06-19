---
id: 0043
title: "Nightly validate driver + ADR-0013 §2 veto-window PR"
status: open
severity: high
group: 0034
depends_on: [0036, 0037, 0038, 0039, 0040, 0041, 0042]
forgejo: 133
links:
  adr: ADR-0013
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The §2 PR/soak/veto window — the headline human-oversight safety net — is unbuilt. New `scripts/scheduled-validate.sh` (sibling of scheduled-import.sh): clone, build, run import→promote→adapt on a branch, batch the whole night into ONE commit, open ONE PR (= the held queue), auto-merge on green AFTER a soak cooldown, ntfy on open + anomaly. systemd timer ordered after the import timer.

Plan ref: `docs/GO_LIVE_HARDENING_PLAN.md` §A1.

## Touches

new `scripts/scheduled-validate.sh` + a systemd unit/timer; reuse forgejo-ci-merge + ntfy plumbing.

## Acceptance

- [ ] A weeknight run opens a single PR (one commit, the run id) that self-merges only after the cooldown.
- [ ] Closing the PR vetoes the batch; `TWICESHY_PAUSE=1` short-circuits before any mutation.
- [ ] OPERATOR STEP: enable the timer (like `twiceshy-import.timer`).
- [ ] Test-first; `make ci` green.

## Notes

Part of the go-live hardening epic (#0034); grounded in ADR-0013 + the 5-agent audit (2026-06-19).
