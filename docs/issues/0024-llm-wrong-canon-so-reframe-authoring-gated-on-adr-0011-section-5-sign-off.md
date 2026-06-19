---
id: 0024
title: LLM-wrong canon + SO-reframe authoring (GATED on ADR-0011 section 5 sign-off)
status: open
severity: medium
group: 0015
depends_on: [0020]
forgejo: 114
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

⛔ **GATED — do NOT build without horia's explicit ADR-0011 §5 sign-off.**

LLM-wrong canon + SO-reframe authoring: use SO / issue-tracker / blog sources
(and the model's training) **only as awareness that a problem class exists** —
never their content. For each problem, **independently re-derive** the fact from
first principles + official docs + execution, and author **our own** description
and **original tests** (the executed test is the licensing firewall). Provenance
= `authored+validated`, not "derived from <url>".

## Notes

**Gate:** §5 is *Proposed*; commercial-pack cleanliness is irreversible → OK for
internal/single-tenant only after sign-off; a real legal review gates any
COMMERCIAL pack. Depends on the harness (0020) being real.

## Acceptance

- [ ] BLOCKED pending sign-off — tracked only; no implementation until then
