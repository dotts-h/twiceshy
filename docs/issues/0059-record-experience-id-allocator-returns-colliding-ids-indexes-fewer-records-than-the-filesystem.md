---
id: 0059
title: record_experience id allocator returns colliding ids (indexes fewer records than the filesystem)
status: open
severity: medium
group: 0008
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: [0060]
  regression:
assets: []
---

## Summary
`record_experience` allocated **exp-0016** for two separate novel drafts during the
2026-06-20 session — but exp-0016 already exists on disk
(`0016-ghsa-23fq-q7hc-993r-...vault.md`). `Index.NextID` is a `MAX(id)+1` read over the
server's indexed `records` table; the live server's index lagged the committed corpus
(no corpus sync — see #0060), so `MAX` was far below the true filesystem maximum and the
allocator handed back an id that was already taken.

## Repro
1. Let the live server's `/data/corpus` drift behind the repo (the default — there was no
   sync; see #0060).
2. Call `record_experience` for a novel record.

Expected: a draft id strictly greater than every committed record id.
Actual: `exp-0016` (collides with the existing `0016-*` record); a second call repeats it.

## Evidence
Session 2026-06-20: worked around by assigning the true next-free ids manually
(exp-0097, exp-0098). `internal/index/index.go:NextID` already documents a *different*
hazard (non-atomic MAX+1 under concurrency, TECH_DEBT M3); this is a distinct failure mode
— a stale index makes MAX+1 collide even single-threaded.

## Notes
Two layers: (a) the root cause (stale live index) is largely removed by the #0060
corpus-sync automation, which keeps the index current; (b) `NextID` should still be robust
against an out-of-date index — e.g. allocate against the source-of-truth corpus tree, or
verify the candidate id is unused before returning. Until fixed, assign ids manually when
the MCP draft id looks low.
