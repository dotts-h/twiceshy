---
id: 0025
title: Hard disk-size cap on the repro work volume (multi-tenant precondition)
status: open
severity: medium
group: 0015
depends_on: [0018]
forgejo: 115
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The #0018 broker runs untrusted repro code with a writable `/work` Docker named
volume. The volume is **disk-backed** because the prepare and execute phases are
separate containers that must share state, and a tmpfs-backed local volume is
re-created empty on every mount (verified on the brain), so it cannot carry the
warmed dependency cache from prepare into execute. Disk-backed means there is
currently **no hard size cap** on `/work`: untrusted code in the execute phase
could `dd` into the volume and fill the host filesystem until killed by the phase
wall-clock (default 3 min).

This is an accepted residual on the single-tenant brain (a self-DoS that is
recoverable — the volume is removed in cleanup, reclaiming the space). It becomes
a **hard requirement before twiceshy runs multi-tenant** (epic 0010), where one
tenant must not be able to exhaust shared host disk.

## Options
- A loopback-backed, fixed-size ext4 image as the volume backing (per run).
- A quota-capable backing FS (xfs project quota / btrfs) + `--opt o=size=` on the
  `local` driver (needs host FS support — the brain's data root may not have it).
- A lightweight disk-usage watchdog: poll the volume's host `_data` size during
  the execute phase and force-kill on cap exceed (needs root to stat the path).

## Notes
- Tracked from the #0018 security review (finder + Gemini both flagged it HIGH for
  multi-tenant). The broker's other host-DoS vectors (unbounded stdout/stderr,
  swap spill, oversized staged files) ARE fixed in #0018.
- Code reference: `internal/repro/broker.go` volume-create comment cites this id.
