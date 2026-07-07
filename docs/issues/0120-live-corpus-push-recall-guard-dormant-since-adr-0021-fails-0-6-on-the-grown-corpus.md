---
id: 0120
title: Live-corpus push recall guard dormant since ADR-0021 — fails 0/6 on the grown corpus
status: open
severity: medium
group:
depends_on: []
forgejo: 521
links:
  adr: docs/adr/ADR-0021-corpus-repo-decoupling.md
  prs: []
  issues: [0117, 0067]
  regression:
assets: []
---

## Summary
`TestPushPrecisionOnLiveCorpus` (internal/eval/eval_livecorpus_test.go, build tag
`livecorpus`) skips whenever `../../experience` is absent — which is always, since
ADR-0021 moved the corpus to the separate twiceshy-corpus repo. The guard has been
dormant and rotted: run today against a fresh clone of the current corpus (3,537
records), push recall is **0.00 (0/6)** — every positive-set trap query is
silenced. Precision (negatives) still holds. Verified as PRE-EXISTING by positive
control: the failure is byte-identical with and without the 0117 IDF asset/gate
changes, so it is corpus drift (and/or eligibility-band drift against the fixed
`pushMaxDF=3` ceiling), not an IDF regression. The exp-0622 vocabulary-exclusion
guard (`TestPushGateExcludesCommonVocabulary`, same tag) passes.

## Repro
1. `git clone <forgejo>/claude/twiceshy-corpus && ln -s <clone>/experience ./experience`
2. `go test -tags livecorpus -run TestPushPrecisionOnLiveCorpus ./internal/eval/`
Expected: recall 1.00 (6/6). Actual: 0.00 (0/6), `got []` for every positive case.

## Fix directions (diagnose first — per exp-3600, make the validity assumption executable)
- Root-cause which stage closes the gate per positive case (discriminative-token
  count vs corroboration vs floor) with the current corpus; the eligible subset
  (validated, trap/fix, non-importer) and per-token df bands have shifted since the
  positive set was authored.
- Wire the guard into something that actually runs (a CI job that clones the corpus
  repo, or a make target the doctor runs) so it can never rot silently again — a
  guard that always skips is indistinguishable from a passing one.
- Recalibrate the positive set or the gate bands only AFTER the root cause is known.

## Notes
Found 2026-07-03 while landing the 0117 IDF asset: the asset PR's acceptance
("live-corpus test stays green") could not be met because the guard was already
red when awakened; 0117 ships with the no-regression control documented instead.
