---
id: 0024
title: LLM-wrong canon + SO-reframe authoring (§5 accepted internal-only; commercial still gated)
status: closed
severity: medium
group: 0015
depends_on: [0020]
forgejo: 114
links:
  adr: ADR-0011 §5
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
- [x] Authoring path: re-derive a problem-class fact from first principles + official
      docs + execution; author our own description + original tests; never ingest/quote
      source text; provenance `authored+validated`
- [x] Records run the full quarantine → judge → soak → human-veto path like any other
- [x] Commercial pack: NOT shipped until a separate real legal review (kept gated)

## Resolution (done 2026-06-23)

The authoring path is the canon [docs/AUTHORING.md](../AUTHORING.md) + the existing
execution-validation engine (#0018 broker, #0020 revalidator, #0029 promote), made
**commercial-safe by construction**. No new engine was needed — authoring is a
discipline over machinery that already shipped.

- **Canon:** `docs/AUTHORING.md` — the single home for §5-clean authoring
  (topic-not-content → re-derive → own description + original tests →
  execution-validate → born quarantined → judge/human-veto), linking ADR-0011 §5,
  the decision memo, ADR-0003 §4, and SCHEMA.md (no duplication).
- **Commercial gate, mechanically enforced:** new `source_license` sentinel
  `none (authored, internal-only)` (`record.SourceLicenseAuthoredInternal`);
  `pack.Classify` maps it to **commercial-ineligible, fail-closed** so authored
  records stay out of commercial packs until a real legal review — the same
  build-time check ADR-0003 §4 uses for copyleft. Schema pattern + Go validator +
  SCHEMA.md updated; covered by tests in `internal/pack` and `internal/record`.
- **Worked example:** [`exp-2753`](../../experience/2026/2753-go-typed-nil-interface-not-nil.md)
  — "a nil pointer returned as a Go error is not nil". Topic widely known publicly;
  description + both tests re-derived from the Go spec and written from scratch; §5
  sentinel, no `source_url`. Ships a positive + negative repro and
  **execution-validates green** through the real gVisor broker (`holds: true`,
  `reproduced_under: [go1.25]`).

Follow-ups filed (not blocking): similarity check (near-verbatim flagger) and an
authoring-scaffold CLI — both referenced from the canon's "Not yet built".
