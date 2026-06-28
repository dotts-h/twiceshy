---
id: 0096
title: Engine binary drifts stale from main — no post-merge redeploy or version guard; scheduled jobs break on new flags
status: closed
severity: high
group:
depends_on: []
forgejo:
links:
  adr:
  prs: [409]
  issues: [0097]
  regression:
assets: []
---

## Summary

The scheduled ops scripts (`scheduled-retro.sh`, `scheduled-import.sh`, `scheduled-validate.sh`) live
in the repo, so a merge updates them instantly — but the engine binary they invoke
(`/home/ori/.local/bin/twiceshy`) is built/installed in a **separate, manual step** and can lag `main`.
When a commit adds a required CLI flag to the binary **and** updates a wrapper to pass it, the wrapper
ships but the binary doesn't → the wrapper passes a flag the stale binary rejects → silent breakage.

## Repro
1. Merge a commit that adds a flag to a `twiceshy` subcommand AND makes a scheduled script pass it
   (real instance: #0096 `-base` on `retro-intake`, commit `221e341`).
2. Do NOT redeploy `/home/ori/.local/bin/twiceshy`.
3. The scheduled job runs the stale binary.

Expected: the job uses a binary consistent with the scripts (or fails loudly that the binary is stale).
Actual: Go `flag.ExitOnError` prints `flag provided but not defined: -base` + usage, exits non-zero;
the drain dies every run and the queue backs up unnoticed (→ corpus stagnation, see exp-2840, #0097).

## Evidence
- Deployed binary was `Jun 26`; `-base` (retro.go:37) landed in `221e341`/#0096 after that → usage dump.
- `Makefile` `build:` is `go build ./...` — **no version embedding, no install/deploy target**.
- Real incident this session: `twiceshy-retro.service` `status=1/FAILURE`, queue at 83 payloads.

## Notes

**Fix (self-healing preflight — chosen):** a shared ops helper (`scripts/lib/ensure-engine-fresh.sh`)
that the scheduled scripts source before using `$BIN`. It compares the engine repo's current commit to
a build-marker recorded next to the binary; if the repo has moved (binary stale), it **rebuilds**
(`go build -o "$BIN" ./cmd/twiceshy`) and updates the marker, logging (and ntfy-alerting) the rebuild.
Self-heals → the exact incident becomes impossible, no human in the loop, no engine code change.

Acceptance:
- A scheduled script run with a stale binary rebuilds it to match the repo before doing work
  (shell-harness test: stale marker → helper rebuilds + updates marker; fresh marker → no rebuild).
- The rebuild is logged/alerted, never silent.
- Belt-and-suspenders (optional, smaller): the wrapper still fails loudly if a rebuild itself fails.

Pairs with **#0097** (detect the silent stall) — this one *prevents* the drift; that one *alarms* when
any capture path stalls regardless of cause.
