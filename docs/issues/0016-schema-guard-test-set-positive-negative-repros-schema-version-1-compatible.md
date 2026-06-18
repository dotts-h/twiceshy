---
id: 0016
title: Schema — guard test-set (positive+negative repros), schema_version-1 compatible
status: open
severity: high
group: 0015
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

Evolve `guard` from a single `repro` path to a **set of tests** per record —
positive (the fix holds) **and** negative (encode dead-ends: prove "don't try Z"
by showing Z still fails), plus variants across inputs/configs/versions
(ADR-0011 §3). More tests = stronger validation + tighter `applies_to` bounds.

**Additive, `schema_version: 1`-compatible:** keep `guard.repro` (single, still
valid) and add an optional list. Old records stay valid; new ones may carry many.

## Touches

- `internal/record/record.go` (`Guard{Repro, GuardingTest}` → + test-list)
- `schema/experience-record.v1.schema.json` (`guard` block ~line 184)
- `docs/SCHEMA.md` (normative; update the `guard` table + an example)

## Acceptance

- [ ] New optional `guard` test-list field (positive/negative tagged), back-compat
- [ ] Validator + JSON Schema accept old single-`repro` AND new list forms
- [ ] Cross-field rule: each listed path must exist (corpus-level, like `repro`)
- [ ] Test-first; `make ci` green; dogfood record if anything bites us
