---
id: 0060
title: Automate corpus sync to the NAS volume (live corpus drifts from the repo; drift crash-looped serve)
status: in-progress
severity: high
group: 0008
depends_on: []
forgejo: 187
links:
  adr:
  prs: []
  issues: [0059]
  regression:
assets: []
---

## Summary
The server rebuilds its index from `/data/corpus` on start, but **nothing kept that
volume in sync with the merged repo**. The live corpus on the NAS volume had drifted
38h+ behind `main`, and it held orphan records (old pandas/setuptools deprecation drafts)
whose ids collided with merged GHSA records — on a restart, `serve` aborted with
`duplicate id exp-0015 ...` and the container crash-looped until the corpus was mirrored
to the repo state. Drift also starved `/push` of newly-merged lessons and produced the
#0059 allocator collision.

## Repro
1. Merge experience records to `main`.
2. Observe the live `/push` does not surface them, and the volume `/data/corpus` differs
   from `main:experience` (orphan/colliding files accumulate).

Expected: the live corpus tracks `main:experience`; a restart always succeeds.
Actual: drift accumulates silently; a restart can crash-loop on a duplicate id.

## Fix (this issue)
`scripts/sync-corpus-to-nas.sh` + a brain-side systemd timer
(`twiceshy-corpus-sync.{service,timer}`, modelled on `twiceshy-import.*`):
- Mirrors `origin/main:experience` to the volume **wholesale** (repo is source of truth,
  ADR-0001 §1), so orphan/colliding records can never re-accumulate.
- Change-gated on the `experience/` git tree SHA (a marker on the volume) — no-op + no
  restart when unchanged; restarts the container only when the corpus actually changed.
- Reads `origin/main` via `git archive` (never a working tree); fail-safe.

## Notes
CI-triggered deploy was rejected: the twiceshy CI runner is gVisor-isolated with no host
docker socket and must not hold NAS deploy creds (ADR-0012). The brain is the trusted
engine (it already runs the importer), so a brain-side timer is the correct seam. Backup of
the pre-mirror drifted corpus: `/data/corpus.drifted.bak.tgz` on the volume — recover any
genuinely-unique orphan lessons into the repo before discarding it.

## Follow-up: hot-reload instead of restart
`serve` now rebuilds its index in place on **SIGHUP** rather than only at startup, so the
sync no longer restarts the container — it `docker kill -s HUP $CONTAINER` to reload. This
removes the restart blip and, crucially, the crash-loop surface: a reload that can't load or
rebuild the new corpus **rolls back** (`index.Rebuild` is one transaction) and keeps the
prior good index serving + alerts (`serve-reload-failed`), where a restart on a bad corpus
would crash-loop. Guarded by `TestRunServeReloadsCorpusOnSIGHUP` (cmd) and
`TestReadyzReflectsHotReloadCount` (server). `docker restart` is now a code-deploy action
only, never a corpus update.
