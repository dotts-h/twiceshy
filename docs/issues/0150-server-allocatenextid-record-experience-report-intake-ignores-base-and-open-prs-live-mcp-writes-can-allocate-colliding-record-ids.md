---
id: 0150
title: server allocateNextID (record_experience/report intake) ignores -base and open PRs — live MCP writes can allocate colliding record ids
status: closed
severity: medium
group: 0064
depends_on: []
forgejo: 611
links:
  adr:
  prs: []
  issues: [0121]
  regression: internal/server/id_allocation_test.go
assets: []
---

## Summary

#0121 hardened every CLI allocation path (`ingest`, `retro-intake`,
`intake-records`, `intake-reports`, `nextid`, `learned`) to allocate above
`max(index, tree, -base, open PR heads)`. The **server** path was out of that
charter and still allocates bare: `allocateNextID` (internal/server/server.go)
calls `ingest.NextID` with index+tree only — no base ref, no open-PR floor. Any
`record_experience` / `report_outcome` write served while corpus PRs are open
can hand out an id an open PR already claims.

## Repro

1. With corpus main at exp-4484 and open validate PRs carrying drafts up to
   exp-4546, call `record_experience` on the live MCP server.
Expected: an id above every state that can merge first (exp-4547+).
Actual: **exp-4485** (observed live 2026-07-10, minutes after #0121 merged) —
collides with the open PRs' range the moment they merge.

## Evidence

- The 2026-07-10 dogfood draft for #0121 itself was allocated exp-4485 by the
  live server; `twiceshy nextid -base origin/main -open-prs` on the same corpus
  said exp-4547. The draft was renumbered by hand before its PR.
- #0121's review sweep flagged this path; deprioritized as out-of-charter, then
  confirmed live the same hour.

## Notes

- Fix shape: thread floors into `allocateNextID` — reuse `ingest.OpenPRMaxID` +
  `ForgejoAPIFromOrigin` (env-first: `TWICESHY_FORGEJO_API/REPO/TOKEN`, which a
  container deployment will need since its corpus checkout may have no usable
  origin). Mind latency/caching: the server is a hot path, unlike the batch
  intakes — a per-write forge scan may need a short-TTL cache, and fail-loud
  needs thought (a forge blip should probably degrade to base-only WITH a loud
  log, not fail the agent's write — the write path is propose-only and the
  corpus dup-id guard still catches collisions at merge).
- Residual same-seconds race stays TECH_DEBT M3 (central reservation) either way.

## Close-out (2026-07-10)

The shared server allocator now uses the index, local tree, configured base ref,
open-PR high-water mark, and process-local allocations. Open-PR resolution is
env-first and cached for 30 seconds; failures are cached and logged while writes
continue against the last-known/base/local floor. Both `record_experience` and
`report_outcome` use the guarded allocator.
