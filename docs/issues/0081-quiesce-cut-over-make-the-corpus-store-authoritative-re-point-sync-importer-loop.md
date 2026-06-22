---
id: 0081
title: Quiesce + cut over: make the corpus store authoritative, re-point sync/importer/loop
status: open
severity: critical
group: 0076
depends_on: [0079, 0080]
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
ADR-0021 phases 3-4 (CRITICAL): pause the importer + promote/adapt timers, drain in-flight PRs to a clean SHA, make the corpus store authoritative (byte-match the snapshot), and re-point the NAS sync + importer + autonomous loop at -corpus <store>. Reversible via the snapshot tag.

## Inherited from #0080
- **Apply guard 1 (gut-check) here**: install the HARD write-lock on the engine-repo `experience/` before snapshotting, and take the authoritative snapshot as the *last* action under that lock (the #0077 baseline tag is not the cutover snapshot).
- **Instantiate the corpus stall alarm**: `scripts/corpus-stall-alarm.sh` is env-configurable — add a brain timer instance with `TWICESHY_FORGEJO_API=…/repos/claude/twiceshy-corpus` once imports flow to the new store.
- **Re-point** the importer/loop's `ENGINE_SHA` consumers and confirm the corpus CI's pinned `ENGINE_SHA` tracks the deployed engine.
