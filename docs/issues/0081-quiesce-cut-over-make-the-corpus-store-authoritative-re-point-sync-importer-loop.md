---
id: 0081
title: Quiesce + cut over: make the corpus store authoritative, re-point sync/importer/loop
status: closed
severity: critical
group: 0076
depends_on: [0079, 0080]
forgejo: 454
links:
  adr: ADR-0021
  prs: []
  issues: []
  regression:
assets: []
---

## Outcome (2026-06-22) — CUT OVER, the corpus store is authoritative + live

Executed the reversible STOP→MOVE→RESTART, verifying each step on a real signal:
- **Quiesced**: stopped + disabled pump/import/validate (won't fire against the write-locked engine).
- **Drained**: closed ~54 open import/validate PRs (re-promotion re-runs; OSV re-fetches). Left #205.
- **Guard 1 write-lock**: engine main `protected_file_patterns: experience/**`.
- **Authoritative snapshot**: tag `corpus-cutover-authoritative-20260622` (engine @ ab45f05, tree de64ef9, 2752 records).
- **Corpus store authoritative**: twiceshy-corpus `experience/` byte-matches de64ef9 (corpus PR #2, CI green).
- **SERVING re-pointed**: NAS sync sources `/home/ori/twiceshy-corpus`; serve `Up (healthy)`, 2752 files — zero disruption.
- **Importer/loop re-pointed** (PR #353): drivers made repo-agnostic + source-free (TWICESHY_FORGEJO_REPO + TWICESHY_BIN + binary-based preflight); data clones re-pointed to the corpus repo; systemd drop-ins set.
- **Stall alarm**: installed + enabled on the brain, watching twiceshy-corpus (the exp-0746 guard — previously authored but never deployed).

Lessons filed (quarantined): exp-2753 (frozen-fixture preflight neutering), exp-2754 (ops-driver repo/source coupling).

## Summary
ADR-0021 phases 3-4 (CRITICAL): pause the importer + promote/adapt timers, drain in-flight PRs to a clean SHA, make the corpus store authoritative (byte-match the snapshot), and re-point the NAS sync + importer + autonomous loop at -corpus <store>. Reversible via the snapshot tag.

## Inherited from #0080
- **Apply guard 1 (gut-check) here**: install the HARD write-lock on the engine-repo `experience/` before snapshotting, and take the authoritative snapshot as the *last* action under that lock (the #0077 baseline tag is not the cutover snapshot).
- **Instantiate the corpus stall alarm**: `scripts/corpus-stall-alarm.sh` is env-configurable — add a brain timer instance with `TWICESHY_FORGEJO_API=…/repos/claude/twiceshy-corpus` once imports flow to the new store.
- **Re-point** the importer/loop's `ENGINE_SHA` consumers and confirm the corpus CI's pinned `ENGINE_SHA` tracks the deployed engine.
