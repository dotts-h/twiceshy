---
id: 0066
title: Agent issue-submission tool (report_issue) + intake
status: open
severity: medium
group: 0064
depends_on: []
forgejo: 252
links:
  adr:
  prs: []
  issues: [0064]
  regression:
assets: []
---

## Summary
Agents have **no surface for half-formed input**. `record_experience` requires a
complete lesson (root_cause, fix, guarding_test); `report_outcome` is tied to an
existing record. So "I hit X but have no fix yet" and "twiceshy itself returned
garbage / crashed" have **nowhere to go** — they are silently lost.

## Approach
- New MCP tool **`report_issue`** with a minimal contract:
  `{ title, description, category: bug|feature|question, related_record_id? }`.
- Routes to a **quarantined intake queue** (mirror the existing report spool
  pattern, ADR-0013 §E1 / #0042) → materialized into **`docs/issues/`** +
  **Forgejo mirror** via the existing `scripts/sync-forgejo.sh`.
- **Dedup** against existing issues at intake (title/embedding similarity offline
  is fine — this is not the hot path).

## Security / invariants
- Inherits bearer auth + rate-limit + content-screen (secrets/PII/harmful) +
  size caps — same as `record_experience` / `report_outcome`.
- Agent-submitted issues are **quarantined / triage-flagged**, never
  auto-actioned; a human (or the triage doctor) promotes them.

## Open questions
- Distinguish "twiceshy-self bug" from "engineering problem I want recorded as a
  future lesson" — route the former to `docs/issues/`, the latter could become a
  `record_experience` draft instead. The `category` field is the discriminator.
- Volume / abuse: rate-limit per token; cap intake-queue depth.

## Notes
Child of epic #0064. Self-contained and parallelizable. Confirmed gap: no MCP
tool, no HTTP endpoint, no intake path for agent-filed issues exists today.
