---
id: 0003
title: Phase 3 — write path + quarantine (record_experience, propose-only)
status: closed
severity: high
group:
depends_on: []
forgejo: 93
links:
  adr: docs/adr/ADR-0008-write-path-persistence-is-a-cli-concern.md
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
The MCP `record_experience` write path (Phase 3): dedup-at-ingest
(`index.Assess` / `ingest.Prepare`), records born `quarantined`, git/PR as the
trust boundary; persistence is a trusted-CLI concern (ADR-0008). **Done** —
propose-only; quarantined records never reach the push channel.

## Notes
Persistence as a git branch + PR is the locked invariant. Promotion to
`validated` is gated on D3 (#0004), not this path.
