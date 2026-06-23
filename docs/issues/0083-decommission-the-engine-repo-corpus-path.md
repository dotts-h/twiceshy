---
id: 0083
title: Decommission the engine-repo corpus path
status: closed
severity: medium
group: 0076
depends_on: [0082]
forgejo:
links:
  adr:
  prs: [371]
  issues: []
  regression:
assets: []
---

## Summary
ADR-0021 phase 6: retire experience/ from the engine repo (leaving only the frozen fixture) once the new home is verified end-to-end.

## Resolution (done 2026-06-23)

Retired the engine-repo corpus path now that twiceshy-corpus is authoritative and
verified end-to-end (#0082):

- **Deleted `experience/`** (2,752 records / ~12 MB) from the engine repo. Serving
  reads the NAS replica synced from twiceshy-corpus, so this git deletion does not
  touch production. `internal/testcorpus/` (the 9-record frozen fixture) stays.
- **Removed the data-only CI shim** — `scripts/ci-data-only.sh` + its test + the
  "Detect data-only change" steps and `if:` conditionals in `ci.yml` & `security.yml`.
  With no `experience/` in the engine repo, `DATA_RE='^experience/'` can never match,
  so the #0078 "Interim D" shim was dead; all CI steps now run unconditionally.
- **Restored `block_on_outdated_branch: true`** in `branch-protection.json` (the #0078
  relaxation existed for ~daily corpus imports that no longer hit the engine repo).
  Re-apply with `scripts/apply-branch-protection.sh`.
- **Migrated the test suite off the in-repo corpus** (the coupling #0079 left in
  `cmd/twiceshy`): command tests now run against the fixture (`testcorpus.Root()`); the
  two corpus-scale-dependent tests (`eval -push` 100/100 and the advisory-gold
  regeneration) moved behind the `livecorpus` tag with skip-if-absent guards; the four
  `LoadCorpus("../..")` livecorpus tests skip cleanly when no corpus is present.
  `make ci` is green without `experience/`.
- **`make eval`** defaults to the fixture, overridable via `CORPUS=<checkout>`, instead
  of the now-empty `-corpus .`.
- **Docs:** CODEBASE_MAP records that the corpus is now the twiceshy-corpus data product.

Closes epic 0076 — this was its last open child. Pre-existing (out of scope): the
`livecorpus`-tagged `TestRetrievePushPrecisionRecall` is red at origin/main too (it
uses the fixture yet expects `exp-0003` to clear the push gate on 9 records); not
introduced here.
