---
id: 0088
title: Corpus coverage — seed engineering traps across the full stack (RN/React/TS/Python/native), not just security advisories
status: open
severity: high
group: 0015
depends_on: []
forgejo: 364
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The corpus is **2,757 records but ~2,700 are imported security advisories** ("upgrade
past the fixed version" — Dependabot/OSV noise, near-zero decision-time value). The
records that actually make agents build faster — engineering traps — number a few dozen
and are **almost entirely Go**. React: 0, React Native: 0, TS frontend: 0, native
(iOS/Android/macOS/Windows) gotchas: 0. The importer scaled record *count* on the wrong
axis; it did not build *usefulness*.

Goal: reach a usable density of validated **engineering traps** per stack cell — the
full target stack is React + React Native + Python/Go/TS backends across
Linux/Windows/macOS/iOS/Android. Rough target: ~40–80 validated traps per cell, ~300–500
total (vs a few dozen, one cell, today).

## Repro
1. An RN/iOS session queried/served twiceshy throughout.
Expected: relevant RN/iOS traps surfaced.
Actual: nothing relevant existed; the session was write-only (it deposited the corpus's
first two RN drafts, both then blocked by bugs — see #0089 and the pii:email screen FP).

## Evidence
Corpus composition (2026-06-23): npm 1330, Packagist 1226, crates.io 593, NuGet 481,
PyPI 438, Go 236, Maven 171, RubyGems 79, everything-else <10 — overwhelmingly advisories.
Field report from the RN session.

## Notes
Two content sources, both currently choked:
- **Dogfood capture** (every session records the traps it hits) — leaky: the RN drafts
  are blocked (#0089 id-collision, the pii:email FP fixed here), agents under-consult
  (fixed by #0087), and `record_experience` had a body-drop bug.
- **Authoring (#0024)** — re-derive a known problem class from first principles + official
  docs + an executed test (the test is the licensing firewall), author *original* traps.
  This is the engine that fills a domain in days not months — and it is **✅ UNBLOCKED for
  internal scope** (ADR-0011 §5 accepted internal/single-tenant, 2026-06-23). Its remaining
  dependency is the harness (#0020). The commercial pack stays gated on a real legal review.

Plan: (1) ~~unblock #0024 (decision)~~ **done — §5 accepted internal-only (2026-06-23);**
(2) fix the capture flywheel — #0089 (done), the pii:email FP (done), #0087 error-scoped
trigger (prototype live), record_experience robustness (remaining); (3) build the authoring
harness (#0020) and run a per-domain seeding campaign using the multi-agent +
execution-validation machinery; (4) extend the #0005 eval into a coverage map per stack
cell. Under #0015 (corpus growth as a live feed).
