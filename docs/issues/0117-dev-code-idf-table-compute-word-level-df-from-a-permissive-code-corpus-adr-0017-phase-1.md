---
id: 0117
title: Dev-code IDF table: compute word-level df from a permissive code corpus (ADR-0017 phase 1)
status: open
severity: medium
group: 
depends_on: []
forgejo:
links:
  adr: docs/adr/ADR-0017-global-idf-push-gate-specificity.md
  prs: []
  issues: [0068, 0111, 0106]
  regression:
assets: []
---

## Summary
ADR-0017 phase 1 implementation. No prebuilt dev-corpus token-frequency table
exists (researched 2026-07-01) — this issue computes a word-level
document-frequency table offline from a permissively-licensed code+docs corpus
sample (e.g. a The Stack v2 stream; license filtered to permissive-only per
ADR-0003's licensing rule), ships it as an embedded asset, and runs it
**alongside** the existing stoplist per ADR-0017 phase 1's explicit plan
("land the global-IDF lookup in addition to the existing stoplist +
`pushMaxDF`"). Word-level, not tokenizer-level, suffices: the push gate's own
hot-path tokenization (`ftsQuery`, `internal/index/index.go:867`) already
splits on `strings.Fields` — a word-level DF table matches the granularity the
gate already computes df at. Supersedes #0111's stoplist bridge once this
path is validated against the negative/positive sets (ADR-0017 phase 2
condition), not before.

## Repro
1. Inspect the push gate's specificity signal for a genuinely rare
   language-of-code token (e.g. `sqlite`) vs an everyday word that is rare
   only because the local corpus is small (ADR-0017's exp-0622 diagnosis).
Expected: a corpus-size-independent global df table exists to answer "is this
rare in the language of code", checked alongside the local-corpus signal.
Actual: no global-IDF asset exists — ADR-0017 remains proposal-only, and only
the hand-maintained stoplist (#0111) stands in for it.

## Evidence
- `docs/adr/ADR-0017-global-idf-push-gate-specificity.md`'s Decision section:
  "Phase 1 — run alongside" the existing stoplist + `pushMaxDF`, gating on
  both, watching for double-filtering; phase 2 (stoplist retirement) is
  conditioned on no regression against both the negative/positive sets and
  #0067's real-traffic labels.
- `internal/index/index.go:867` (`ftsQuery`) and `:116`/`:575` (existing
  `strings.Fields`-based tokenization elsewhere in the same file) confirm the
  gate already operates at word granularity — a word-level DF table is the
  matching signal, not an under- or over-fit to a different tokenizer.
- ADR-0017's own make-or-break constraint: the background corpus must be
  dev/code, not general English, or the exp-0622 bug (common dev words scored
  as rare) re-appears at the global level instead of the local one.

## Acceptance
- An offline precompute produces a word-level document-frequency table from a
  permissively-licensed code+docs corpus sample; licensing is checked per
  source before inclusion (ADR-0003 §4's facts-only/attribution discipline
  extends to this bulk asset the same way it does to individual records).
- The table ships as an embedded asset with a documented size budget.
- The push gate consults the global-IDF table **alongside** (not instead of)
  the existing stoplist + `pushMaxDF`; a `TestPushGate*` guard covers the new
  path, and the existing live-corpus precision/recall test
  (`internal/eval/eval_livecorpus_test.go`, the exp-0622 guard) stays green.
- Double-filtering (a genuine query silenced by both signals) is measured and
  reported, per ADR-0017's explicit phase-1 watch item.

## Notes
This is phase 1 only — "run alongside," never a replacement. Retiring #0111's
stoplist (ADR-0017 phase 2) is a separate, later decision gated on the
no-regression evidence above; this issue does not remove the stoplist.
