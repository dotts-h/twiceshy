---
id: 0018
title: Sandbox broker + watchdog — ephemeral gVisor, hardcoded policy, two-phase egress, guaranteed cleanup
status: open
severity: critical
group: 0015
depends_on: [0017]
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The sandbox **broker** (ADR-0011 §4 must-haves A+B): create an **ephemeral**
gVisor container per repro under a **HARDCODED policy** the record can't
influence — non-root, read-only rootfs, dropped caps, resource caps (mem / cpu /
pids / wall-clock), **named volume not bind** (exp-0004). **Two-phase:**
`prepare` = allowlisted egress (fetch pinned deps), `execute` = `--network=none`.
The **watchdog** guarantees termination + cleanup: hard-timeout kill + a reaper
that removes labeled containers even on panic/crash. No leaked containers, ever.

## Touches

`internal/repro` (broker); injected into the doctor as a seam, like D2's `EOLSource`.

## Acceptance

- [ ] Broker runs a test in runsc, returns exit/status, tears down deterministically
- [ ] Policy hardcoded (never from the record); execute phase has no network
- [ ] Watchdog: timeout → kill + cleanup; reaper clears orphans; race-tested
- [ ] Unit tests stub the container; one integration test against real runsc
- [ ] Depends on 0017 (runsc + base images)
