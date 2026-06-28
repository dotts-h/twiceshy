---
id: 0097
title: Capture-pipeline stalls are silent — watch real-experience inflow / queue-drain, not total record count
status: in-progress
severity: high
group:
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: [0096]
  regression:
assets: []
---

## Summary

The retro capture path (`twiceshy-retro.timer` → `scheduled-retro.sh` → drains
`/home/ori/twiceshy-retro-queue`) failed on every run for ~2 days and the queue backed up to 83
payloads with **no alarm**. `twiceshy-growth-watchdog.sh` watches `status: validated` count growth on
main and the timers `(validate, drain-merge, stall-alarm)` — but **the CVE importer keeps that total
rising**, so it stayed green, and **the retro timer + queue aren't watched at all**. The one signal
that mattered — a capture queue that stopped draining — had no monitor.

## Repro
1. Break the retro drain (e.g. stale binary, #0096) so `twiceshy-retro.service` exits non-zero.
2. The queue accumulates; the importer keeps total record count rising.
3. `twiceshy-growth-watchdog.sh` sees the total still climbing → stays green.

Expected: an alarm fires when a capture path stalls (queue not draining / unit failed).
Actual: silent for days; discovered only by manual inspection.

## Evidence
- `twiceshy-growth-watchdog.sh` TIMERS = `(twiceshy-validate.timer twiceshy-drain-merge.timer
  twiceshy-stall-alarm.timer)` — **no `twiceshy-retro.timer`**, and it never inspects the retro queue.
- The queue had 83 undrained files dated Jun-26; the drain unit was in `failed` state.

## Notes

**Fix (extend the existing growth-watchdog — chosen):** add two checks to
`scripts/twiceshy-growth-watchdog.sh`, reusing its ntfy path:
1. **Queue-drain health** — alert if `$TWICESHY_RETRO_QUEUE` (default `/home/ori/twiceshy-retro-queue`)
   holds payload files older than `MAX_QUEUE_AGE_HOURS` (default a few drain cycles, e.g. 12h): a queue
   that isn't draining is a dead capture path.
2. **Retro unit health** — add `twiceshy-retro.timer` to the watched `TIMERS` (and flag the
   `.service` in `failed` state).

Acceptance (shell-harness tests, `make test-scripts`):
- A queue dir with a file older than the threshold → watchdog emits an alert (mockable ntfy).
- A fresh/empty queue → no alert.
- `twiceshy-retro.timer` is present in the watched-timer set.
- Existing growth/timer checks unchanged.

Pairs with **#0096** (prevent the drift that caused this instance) — this one catches a stalled capture
path regardless of cause.
