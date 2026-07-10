---
id: 0148
title: Validate driver: preserve the journaled promote batch across an out-of-band SIGTERM (reboot/deploy) so partial work survives the next run's git reset --hard
status: closed
severity: medium
group: 0034
depends_on: [0123]
forgejo: 591
links:
  adr:
  prs: []
  issues: []
  regression: scripts/scheduled-validate.test.sh
assets: []
---

## Summary

Split from #0123 (the driver half of "validate-run timeout poisons the backlog").
#0123 fixed the two acute defects: a judge **transport error is now `deferred`, not
`held`** (no 168h cooldown poisoning), and the loop **breaks cleanly on
`context.Canceled`** leaving unreached records untouched; the repo-tracked
`TimeoutStartSec` is baked to 18000s (5h) so a normal sweep is never SIGTERMed
mid-run. That closes the recurring freeze.

What remains is defensive durability: when a SIGTERM arrives from an **out-of-band**
source the timeout bump does not cover — a reboot, a `systemctl stop`, a deploy, an
OOM kill — it can still land between promote persisting its ~60 promoted-record edits
to the run-branch working tree and the driver's commit/PR step. The next run's
`git reset --hard origin/main` then wipes those uncommitted edits (the 2026-07-06
batch was salvaged by hand as corpus PR #163). The promoted work is lost even though
the run journal (`runs/*.journal.json`, #0054, runner-local so it survives the reset)
recorded it.

## Repro
1. Start `twiceshy-validate` with a large eligible quarantine so a sweep is in flight.
2. `systemctl stop twiceshy-validate` (or reboot) after promote has persisted some
   records but before the commit step.
Expected: whatever promote already journaled is committed (or otherwise survives) so
the batch degrades to a smaller committed batch, not lost work.
Actual: the uncommitted promoted-record edits are discarded by the next run's
`git reset --hard`.

## Evidence

- #0123 engine fix + timeout bump: PRs from #0123 (deferred outcome, ctx-cancel break,
  `scripts/twiceshy-validate.service` TimeoutStartSec=18000).
- `runs/*.journal.json` already persists each decision incrementally (#0054) and is
  gitignored, so it is NOT removed by `git clean -fd experience/ runs/` — the recovery
  source is already on disk.

## Notes

Two candidate approaches (from #0123 Notes), both needing a **test harness for
`scheduled-validate.sh` first** (there is none today — TDD gate before touching the
data-safety-critical `abort()`/reset path in the production loop):

- **SIGTERM trap in the driver:** `trap` SIGTERM to commit+push whatever promote
  journaled before exiting (the manifest is the rollback boundary anyway), so a kill
  degrades to a smaller committed batch.
- **`timeout`-wrapped promote:** run promote under `timeout <budget>` with a budget
  below `TimeoutStartSec`, and treat the `timeout` exit code (124) as "partial batch —
  commit what persisted" rather than routing it to `abort()` (which deletes the branch).
- **Startup recovery guard:** before `git reset --hard`, detect a non-empty leftover
  `validate/*` branch / uncommitted experience edits from a killed run and commit+PR
  them (recover) instead of destroying — "never silent again" for lost promotions.

Related: #0123 (parent), #0054 (run-journal resume cursor — the recovery source),
#0084 (hold cooldown), #0122 (promotions-liveness alarm).

## Close-out (2026-07-10)

The driver now preserves validation-owned `experience/` and `runs/` changes on
SIGTERM as a scoped commit and pushed recovery PR marked for manual review. A
startup guard recovers dirty/ahead `validate/*` branches before destructive
reset/clean hygiene, while dry-runs refuse external writes and ordinary aborts
retain their prior cleanup behavior. The hermetic regression covers both paths
against local bare remotes.
