---
id: 0004
title: Doctors — background store-hygiene jobs (D1–D5), delta-only
status: open
severity: high
group: 0008
depends_on: [0007]
forgejo:
links:
  adr: docs/adr/ADR-0001-architecture.md
  prs: []
  issues: [0002]
  regression:
assets: []
---

## Summary
The background jobs that keep the store honest — **incremental deltas only,
never whole-store rewrites** (context collapse). They run on the brain engine;
LLM-arbitrated steps use local Ollama or a Haiku-class API, batch/overnight
(ADR-0001 §7, §9). **D3 is the one that unblocks the push channel**: it promotes
`quarantined → validated`, so it is the first doctor to build.

## Children (file when broken down; D3 first)
- **D3 revalidation (build first, the novel one):** re-execute each record's repro
  in an **isolated sandbox container** (brain) on a schedule and on dependency
  bumps; SWE-bench fail-to-pass discipline. Promotes quarantine→validated; demotes
  if a repro stops failing pre-fix.
- **D1 dedup/reconcile:** LLM arbitrates ADD/UPDATE/SUPERSEDE/NOOP for a candidate
  vs its top-k similar records.
- **D2 staleness:** cross-check `applies_to` against live versions/docs
  (Context7-style); fed by endoflife.date + Renovate/Dependabot bump events.
- **D4 lifecycle:** reinforce helpful records, decay never-hit ones — archive,
  never delete; beware evicting rare-but-critical traps (salience beats recency).
- **D5 abstraction:** induce `convention` cards from clusters of related episodes;
  episodes remain as evidence links.

## Acceptance
- [ ] D3 promotes ≥1 imported (#0007) record to `validated` via a real sandbox
      fail-to-pass run; guard test ships with it (repo hard rule).
- [ ] All doctors operate by delta; none rewrites the whole store.

## Notes
Depends on #0007 (records to validate/keep fresh). D3's sandbox runner is the
seam the future "check issues in isolated containers" enhancement builds on.
