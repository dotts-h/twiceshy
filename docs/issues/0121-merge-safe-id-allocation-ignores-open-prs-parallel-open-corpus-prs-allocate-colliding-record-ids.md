---
id: 0121
title: Merge-safe ID allocation ignores OPEN PRs — parallel-open corpus PRs allocate colliding record IDs
status: open
severity: medium
group: 0064
depends_on: []
forgejo: 522
links:
  adr: docs/adr/ADR-0021-corpus-data-product-split.md
  prs: []
  issues: [0105]
  regression:
assets: []
---

## Summary

`ingest`/`retro-intake` allocate new record IDs merge-safely against `-base origin/main` —
but only against **main**. Any drafts sitting on **open, unmerged PRs** are invisible to the
allocator, so two PRs opened while the first is still unmerged both allocate the same ID
range. Discovered while draining the 0105 backlog: with retro automerge off, 26 PRs piled
up over 5 days and **549 of 625 records carried colliding IDs** — whole clusters of PRs
sharing identical ranges (#69/#71 both 3197–3221; four PRs each at 3404+, 3481+, 3715+,
3846+), plus collisions against records main gained later from imports.

## Impact

- Any merge of the second PR in a cluster breaks main's corpus guard (dup-id) — the CI-green
  state of an open PR is stale the moment a sibling merges.
- The failure is invisible at PR creation: each PR is green against the main it saw.
- Automerge-on-green (import/retro default) narrows the window to minutes but does NOT close
  it: two drains/imports racing, or any PR held open (quality holds, red CI, review), reopens it.

## Repro

1. Open corpus PR A allocating exp-N..N+k (green, leave unmerged).
2. Run a second drain/import → PR B allocates the same exp-N..N+k.
3. Merge A, then update+CI B → dup-id failure (or, without the guard, a silently forked ID).

## Fix directions (pick one, smallest first)

- (a) Allocator scans open PR heads via the Forgejo API and allocates above
  `max(main, open PR heads)` — closes the race for the common single-writer case.
- (b) Central ID reservation (a counter file/endpoint bumped atomically) — heavier, closes
  every race.
- (c) Give up dense IDs: ULID/hash-suffixed record IDs — no coordination, but breaks the
  human-friendly exp-NNNN convention and every consumer that sorts on it.

The 0105 drain worked around it once (consolidated PR #132 re-numbered 549 records above
main max); the workaround does not scale to routine operation.
