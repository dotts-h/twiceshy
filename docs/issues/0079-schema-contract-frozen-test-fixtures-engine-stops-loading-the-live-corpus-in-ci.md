---
id: 0079
title: Schema contract + frozen test fixtures (engine stops loading the live corpus in CI)
status: open
severity: high
group: 0076
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
ADR-0021 phase 1: the engine declares its supported schema_version (the engine<->corpus contract); replace live-corpus CI loads (the #0074 gold-set golden test, eval) with a small frozen fixture so code CI no longer depends on the live corpus (also de-flakes the golden test).
