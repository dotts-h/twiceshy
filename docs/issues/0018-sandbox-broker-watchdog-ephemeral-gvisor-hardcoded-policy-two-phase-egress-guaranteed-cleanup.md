---
id: 0018
title: Sandbox broker + watchdog — ephemeral gVisor, hardcoded policy, two-phase egress, guaranteed cleanup
status: closed
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

- [x] Broker runs a test in runsc, returns exit/status, tears down deterministically
- [x] Policy hardcoded (never from the record); execute phase has no network
- [x] Watchdog: timeout → kill + cleanup; reaper clears orphans; race-tested
- [x] Unit tests stub the container; one integration test against real runsc
- [x] Depends on 0017 (runsc + base images)

## Resolution (done 2026-06-18)

`internal/repro` broker + watchdog + reaper, built TDD with a `commandRunner`
seam (unit tests stub the docker CLI; integration tests drive real runsc, gated
by `TWICESHY_REPRO_INTEGRATION=1` so the socketless CI runner skips them).

Hardcoded policy on every phase (record can't influence it): `--runtime=runsc`,
non-root `65534`, `--read-only`, `--cap-drop=ALL` (+ only `CHOWN` on the trusted
populate step), `--security-opt=no-new-privileges`, `--memory`/`--memory-swap`
(swap off), `--cpus`, `--pids-limit`, per-phase wall-clock, `/tmp` tmpfs
(nosuid,nodev,noexec). Writable mount is a per-run **named volume** at `/work`
(never a host bind, exp-0004), removed in cleanup. **Two-phase:** `prepare` runs
a *trusted* caller command on the default bridge (gVisor DNS gotcha, exp-0016) to
warm pinned deps; `execute` runs the untrusted script with `--network=none`.
**Watchdog:** per-phase timeout → label-sweep force-kill; deferred cleanup +
`Reaper` (standing backstop) remove every labelled container + volume even on
panic. Verified on real runsc: non-root, gVisor kernel, offline execute, R/O
rootfs, timeout-kill, zero leaks.

Security review (finder subagent + Gemini): isolation core confirmed sound
(policy-injection impossible; execute offline). Fixed the host-side DoS vectors
they/­I found — unbounded stdout/stderr capture (now 1 MiB-capped), swap spill
(`--memory-swap == --memory`), oversized staged files (8 MiB cap). The one
residual — no hard disk-size cap on the disk-backed `/work` (a tmpfs volume
can't carry state across the phase containers) — is filed as **#0025** and in
TECH_DEBT; it is an accepted single-tenant residual, a hard precondition before
multi-tenant.
