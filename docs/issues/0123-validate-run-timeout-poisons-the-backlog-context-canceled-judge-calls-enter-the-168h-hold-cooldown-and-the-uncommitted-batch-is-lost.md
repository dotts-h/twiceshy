---
id: 0123
title: Validate-run timeout poisons the backlog: context-canceled judge calls enter the 168h hold cooldown and the uncommitted batch is lost
status: open
severity: high
group: 
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

When the systemd service timeout (TimeoutStartSec) SIGTERMs a validate run
mid-sweep, two independent defects compound (observed 2026-07-06,
run-20260706T093333Z, the first run after the exp-4454 judge-schema fix):

1. **Context-canceled judge calls enter the 168h hold cooldown.** The canceled
   context makes every not-yet-judged record fail its judge call with
   `judge: call gpt-oss:20b: Post "http://localhost:8723": context canceled`,
   which the promote loop records as `held` — 3,043 of 3,238 holds in that run
   were this artifact, not verdicts. One timeout therefore re-freezes the whole
   eligible backlog for a week. A transport-layer failure (context canceled,
   deadline exceeded, connection refused) says nothing about the record and
   should NOT start its hold cooldown; only a judge *decline* (a real verdict)
   should.

2. **The uncommitted batch is destroyed.** The SIGTERM lands between promote
   finishing (manifest + 60 promoted record edits on the run branch working
   tree) and the driver's commit/PR step, and the next run's `git reset --hard`
   wipes it. The 2026-07-06 batch had to be salvaged by hand (corpus PR #163).

## Repro
1. Set the service TimeoutStartSec below the time a full sweep needs (it was
   3600s vs ~3.5h needed at ~8s/record to reach TWICESHY_MAX_PROMOTIONS=200).
2. Start twiceshy-validate with a large eligible quarantine.
Expected: unreached records left untouched (like the clean throughput-cap
break); whatever promote already wrote is committed or at least survives.
Actual: every unreached record held with a `context canceled` reason for 168h;
the promoted records' edits are lost to the next run's reset --hard.

## Evidence

- `run-20260706T093333Z-promote.json` (committed on the corpus run branch,
  merged via corpus PR #163): counts `{held: 3238, ineligible: 1048, promoted: 60}`;
  1,646 advisory + 1,397 prose holds read `... Post "http://localhost:8723":
  context canceled`.
- systemd: `twiceshy-validate.service: Failed with result 'timeout'` at exactly
  start+3600s.
- Ops mitigations applied 2026-07-06 (both should become unnecessary): stripped
  the 3,064 transport-artifact entries from `runs/promote.holds.json`; drop-in
  override raising TimeoutStartSec to 18000 (5h) so the 200-promotion clean cap
  is reachable.

## Notes

Suggested fixes (engine + driver, separable):

- **Engine (the real fix):** in the promote loop, distinguish judge transport
  errors from judge declines. On `context.Canceled` specifically, stop the sweep
  cleanly (same path as the throughput cap: break, journal "run interrupted",
  leave the remainder untouched). Other transport errors (timeout, refused) may
  still journal `held` but must not write the hold-cooldown entry.
- **Driver:** `scheduled-validate.sh` should trap SIGTERM and commit+push
  whatever promote already journaled before exiting (the manifest is the
  rollback boundary anyway), so a timeout degrades to a smaller batch instead of
  lost work. Alternatively run promote under `timeout -k` inside the script with
  a budget below TimeoutStartSec, keeping the commit step reachable.
- Related: #0084 (hold cooldown), #0122 (promotions-liveness alarm — would have
  caught the frozen week this run was recovering from), exp-4454 (the schema
  drift that caused the original freeze).
