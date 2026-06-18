---
id: 0001
title: Phase 0 — seed the corpus from our own repos
status: closed
severity: medium
group:
depends_on: []
forgejo:
links:
  adr: docs/adr/ADR-0001-architecture.md
  prs: []
  issues: [0007]
  regression:
assets: []
---

## Summary
Seed `experience/` with the first records, distilled from our own
REGRESSIONS/ADRs/build experience, so the Phase 1 read path has something to
index (ADR-0001). **Done** — three dogfooding records ship
(`experience/2026/0001..0003`).

## Notes
Closed but ongoing in spirit: #0007 (corpus importer) extends Phase 0 from our
own repos to external license-clean sources. The corpus is the moat (CONTEXT.md).
