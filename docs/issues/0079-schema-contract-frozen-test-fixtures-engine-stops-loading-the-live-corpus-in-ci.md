---
id: 0079
title: Schema contract + frozen test fixtures (engine stops loading the live corpus in CI)
status: closed
severity: high
group: 0076
depends_on: []
forgejo: 452
links:
  adr: ADR-0021
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
ADR-0021 phase 1: the engine declares its supported schema_version (the engine<->corpus contract); replace live-corpus CI loads (the #0074 gold-set golden test, eval) with a small frozen fixture so code CI no longer depends on the live corpus (also de-flakes the golden test).

## Outcome (2026-06-22) — DONE

Engine CI no longer loads the live `experience/` corpus — the prerequisite that lets the
corpus be physically removed from this repo at phase 4.

- **Frozen fixture + helper.** New `internal/testcorpus` package: `Root()` returns the bundled
  fixture at `internal/testcorpus/corpus/experience/` — **9 records copied verbatim from the
  live corpus** (exp-0001..0006, 0017, 0043 validated + exp-0046 quarantined) plus their repro
  dirs. `record.LoadCorpus(testcorpus.Root())` satisfies the strict integrity checks.
- **Repointed to the fixture** (engine-logic tests — parser/marshal/schema/loader/index/serve):
  `internal/record/{record,schema,marshal,guard_repros}_test.go`, `internal/index/{index,fuzz}_test.go`,
  `internal/server/{server,usage,logging}_test.go`. Assertions unchanged.
- **Moved behind `//go:build livecorpus`** (corpus-relative *behaviour* guards — BM25/df is
  corpus-scale-dependent, so they belong in the corpus repo's CI, runnable here via
  `make test-livecorpus`): `eval.TestPushPrecisionOnLiveCorpus`,
  `index.TestPushGateExcludesCommonVocabulary`, `index.{TestRetrievePushPrecisionRecall,
  TestRetrievePushTraced,TestRetrievePushExcludesQuarantined}`,
  `server.TestTelemetryRecordsPushGateDecision`, `doctor.TestStaleness_RealCorpusNotFalseFlagged`.
  Coverage of the moved code paths is preserved by a deterministic fixture test
  (`TestPushGateDiscriminativeTokensOnFixture`); `make cover-check` stays green at the 80% floor.
- **Schema-version contract made explicit** (gut-check guard 2, engine side): the engine already
  declares `const record.SchemaVersion = 1` and rejects mismatches; added
  `TestSchemaVersionContractRejected` asserting Parse + LoadCorpus reject `schema_version+1` with a
  clear error. The *corpus-CI-checks-the-deployed-engine-version* half of guard 2 is #0080's scope.

Verified independently: `make lint`, `go test -race ./...`, `make cover-check` (80.0%),
`scripts/check-workflows.sh` all green; all fixture records confirmed byte-identical to live.
