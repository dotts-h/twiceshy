---
id: 0024
title: LLM-wrong canon + SO-reframe authoring (§5 accepted internal-only; commercial still gated)
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

✅ **UNBLOCKED for internal scope (2026-06-23, horia accepted ADR-0011 §5 for
internal/single-tenant use only).** Build the internal authoring path. The
**COMMERCIAL pack stays gated** on a real legal review — do not ship SO-derived
records in a commercial pack.

LLM-wrong canon + SO-reframe authoring: use SO / issue-tracker / blog sources
(and the model's training) **only as awareness that a problem class exists** —
never their content. For each problem, **independently re-derive** the fact from
first principles + official docs + execution, and author **our own** description
and **original tests** (the executed test is the licensing firewall). Provenance
= `authored+validated`, not "derived from <url>".

## Notes

**Gate cleared (internal):** ADR-0011 §5 is **Accepted for internal/single-tenant**
(2026-06-23). The remaining dependency is the harness (#0020) being real — authoring
needs the execution-validation engine so each authored record ships original, executed
tests (the licensing firewall). The **commercial** pack is still gated on a real legal
review; that gate is unchanged.

## Acceptance

- [x] ADR-0011 §5 signed off for internal/single-tenant scope (horia, 2026-06-23)
- [ ] Authoring path: re-derive a problem-class fact from first principles + official
      docs + execution; author our own description + original tests; never ingest/quote
      source text; provenance `authored+validated`
- [ ] Records run the full quarantine → judge → soak → human-veto path like any other
- [ ] Commercial pack: NOT shipped until a separate real legal review (kept gated)
