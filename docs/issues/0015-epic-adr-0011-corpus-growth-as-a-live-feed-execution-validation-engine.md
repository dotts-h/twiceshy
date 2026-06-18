---
id: 0015
title: Epic: ADR-0011 — Corpus growth as a live feed + execution-validation engine
status: open
severity: high
group: 
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

Turn the corpus from a **static seed into a live, validated feed** (ADR-0011).
Two things become real: continuous live ingestion, and an execution-validation
engine that makes "validated" mean "we ran it and it holds." Positioning: the
execution-validated, fresh, negative-knowledge **pre-flight landmine check** for
coding agents.

**ADR status:** Proposed — phases 1–3 proceed now; phase 4 + any commercial pack
are gated on horia's sign-off (§5 / CONTRIBUTION_TERMS).

## Phasing (children)

Disjoint seams → **parallel lanes** (`internal/repro` ⟂ `internal/ingest` ⟂
`internal/screen` ⟂ `internal/record`+`schema`):

1. **Validation harness** (the moat): 0016 schema test-set · 0017 gVisor infra ·
   0018 broker+watchdog · 0019 screen-of-repro · 0020 revalidate doctor.
   Preconditioned on the 3 hardening must-haves (0017 → 0018; 0019).
2. **Live OSV importer** (the feed): 0021 fetch+distill+quarantine · 0022 schedule.
3. **Deprecations + codemods**: 0023 deps.dev/endoflife live.
4. **SO-reframe canon**: 0024 — GATED on §5 sign-off + the harness.
5. **Eval (#0005)** runs alongside — existing issue, not refiled.

## Definition of done

Record count climbs **on its own** (scheduled importer) and records are
**promoted by execution** (harness), served over MCP. Data pours in.

## Notes

Grounding: ADR-0011, CORPUS_GROWTH_RESEARCH, PLATFORM_RESEARCH, SECURITY_ANALYSIS.
