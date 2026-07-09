---
id: 0111
title: Extend the push stoplist with a static dev-vocabulary list (ADR-0017 cheap proxy)
status: closed
severity: low
group: 0106
depends_on: []
forgejo: 477
links:
  adr:
  prs: [576]
  issues: [0106, 0068]
  regression:
assets: []
---

## Summary
Bridge item toward ADR-0017 (global dev/code IDF, #0068 closed as
proposal-only): extend `pushStopwords` (`internal/index/index.go:98`,
`commonWords`, `:113`) with a static, embedded top-N common-dev-vocabulary list
so **ordinary words that happen to be rare in today's corpus** (`rename`,
`button`, `email`, `deploy`, `menu`, `login` — all measured discriminative at
df∈[1,3] against the live corpus, epic #0106) cannot open the gate even as corpus
composition shifts. Explicitly the cheap proxy, not the endgame — superseded when
ADR-0017's real IDF lands. Backlog: #0107 + #0108 are the structural fix for this
epic; this hardens the tail the df-inversion leaves after them.

## Repro
1. Sample everyday dev vocabulary against the live validated `records_fts` (the
   47-word sample, epic #0106's evidence).
Expected: none of these ordinary words are discriminative regardless of corpus
composition.
Actual: 17 of 47 sampled everyday dev words are discriminative (df∈[1,3]) at the
current 990-record corpus, including `rename`, `button`, `email`, `deploy`,
`menu`, `login`.

## Evidence
- `internal/index/index.go:98` (`pushStopwords = wordSet(commonWords)`) and the
  `commonWords` const (`:113` onward) — English function words + existing
  common software/web/ops/data vocabulary; the sampled leaking words are not yet
  in this list.
- Epic #0106's 47-word sample (17/47 discriminative) and its root-cause finding
  that the df gate inverted as validated grew 30→990 (~95% OSV advisories),
  making ordinary words look rare.
- `internal/index/index.go:96-97`'s own comment: the stoplist is meant to be
  "grow[n] — with both guards — as new common words surface"; this issue is
  exactly that growth, sourced from a static top-N list instead of one-off
  patches.

## Acceptance
- The sampled ordinary-word list (the 17/47 discriminative words, plus a
  representative top-N common-dev-vocabulary set) is no longer discriminative
  after the extension.
- The positive on-topic set (`eval.PushPositives()`, `internal/eval/eval.go:191`)
  is unaffected — watch double-filtering (ADR-0017 Phase-1's own concern: a
  genuine query silenced because a real topical token collides with the static
  list).
- `TestPushGateExcludesCommonVocabulary` (referenced by
  `internal/index/index.go:96`) is extended to cover the new entries; the
  live-corpus precision/recall guard (`TestPushPrecisionOnLiveCorpus`,
  `internal/eval/eval_livecorpus_test.go`) stays green.

## Notes
Explicitly a hand-tuned, never-provably-complete patch (ADR-0017's own critique
of the status-quo stoplist approach) — kept in scope here only as a stopgap
because #0107/#0108 are the structural fix and ADR-0017's corpus-sourced global
IDF is still the endgame, not yet built (#0068 closed as proposal-only). Do not
let this issue's scope creep toward re-deriving IDF locally — that is exactly the
exp-0622 bug ADR-0017 rejects as Option 2.
