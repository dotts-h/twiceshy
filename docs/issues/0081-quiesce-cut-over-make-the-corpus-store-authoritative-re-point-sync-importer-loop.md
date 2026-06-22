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
