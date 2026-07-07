---
id: 0138
title: Declared middleware pipeline — stage list with checked requires/after constraints, per-request state, signup joins the chain
status: closed
severity: medium
group: 0124
depends_on: []
forgejo:
links:
  adr: ADR-0033
  prs: []
  issues: [0131]
  regression:
assets: []
---

## Summary
The HTTP middleware chain in `server.New` was built as nested closure calls across three separate statements, with correctness relying on five comment-enforced ordering invariants. The sprint history showed this was fragile in practice: three reorders in one week led to multiple escaped bugs, including a quota-debit misordering (#0131). Additionally, the access log outside auth relied on a mutable `tenantHolder` side-channel to bridge context values upstream, and `/signup` was hand-wired to skip critical timeout and body-cap stages.

The fix introduces a declared stage list validated at server construction (`buildChain`) enforcing `requires` and `after` constraints, replaces `tenantHolder` with a single per-request context state (`reqState`), and includes `/signup` in the declared pipeline (gaining timeout and transport body limit capabilities). A suite of chain-contract integration tests verifies correct ordering and behavior under limits.

## Repro
1. Swap `daily-quota` above `global-rate-limit` in the `authedStages` declaration inside `server.go`.
2. Attempt to construct a new server using `server.New()`.

Expected:
`server.New()` fails at construction with a validation error: `stage "daily-quota" must run after stage "global-rate-limit", but "global-rate-limit" is declared later or at the same position`.

Actual before this change:
The server starts up successfully, but silently mischarges quotas when a request gets globally rate limited (the #0131 bug).

## Evidence
- The validation error occurs on misordered startup and is caught by unit tests (`TestBuildChain_Validation` in `pipeline_test.go`).
- The new `TestChainContract` in `chain_contract_test.go` verifies correct ordering of auth vs global limits, and limits vs quota debits.

## Notes
External behavior remains byte-identical except that `/signup` now gains the 30-second request timeout limit and the 256KiB transport body size limit.
