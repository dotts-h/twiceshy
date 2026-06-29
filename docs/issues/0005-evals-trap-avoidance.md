---
id: 0005
title: Trap-avoidance eval suite — memory on/off regression for the store
status: in-progress
severity: medium
group: 0008
depends_on: []
forgejo: 95
links:
  adr: docs/adr/ADR-0001-architecture.md
  prs: [419, 423]
  issues: [0064, 0067, 0105]
  regression:
assets: []
---

## Summary
The project's regression suite for the store itself (Phase 5): walk an agent
toward each recorded trap with memory **on vs off**, and score avoidance plus
steps/tokens. Publishable novelty — no published suite measures this
(ADR-0001 §8).

## Scope
- [x] Harness: drive an agent toward each `trap`/`dead-end` record, memory on/off. *(slice 2 — `ModelRunner` over a real off-pool model; PR #423)*
- [x] Metrics: avoidance rate, steps-to-solution, tokens; per-record and aggregate. *(slice 2 — `Report` aggregates both arms; executable `BrokerVerifier` decides avoidance)*
- [x] Wire into `make ci` (or a separate target) as the store's regression gate. *(`make eval` target; report-only, not blocking — recall shifts as the corpus grows)*
- [x] Report the near-miss failure mode explicitly (does a related-but-wrong card hurt?). *(near-miss rate + per-case wrong-card reporting)*

## Progress (slice 1, 2026-06-19)

`internal/eval` + `twiceshy eval` ship the **retrieval-effectiveness** slice: the
cheap, deterministic precondition for the agent eval. It drives the same
validated-only pull path an agent uses, with queries taken from each behavioral
record's error signatures (the text an agent sees) + symptom summary, and reports
**recall@k, MRR, near-miss rate** per-case and aggregate. No LLM budget.

First run on the live corpus (18 cases over the 6 validated behavioral records):
**recall@3 = 100%, MRR = 0.972, near-miss = 5.6%**. The one near-miss is genuine
ambiguity, not a defect: the bare signature `"permission denied"` surfaces
exp-0004 (NAS bind-mount perm-denied) above exp-0017 (noexec-TMPDIR perm-denied)
— both legitimately match. Evidence that retrieval works on the validated set.

**Slice 2 (remaining):** the GitChameleon-style agent-task eval — does the
retrieved card change task success / steps / tokens (memory on vs off).

## Progress (slice 2 — harness foundation, 2026-06-28, PR #419)

`internal/agenteval` ships the agent-task eval **harness**: `AgentRunner` + `Verifier`
injectable seams, `Run` drives every `TaskCase` through BOTH arms (memory off = no card,
memory on = the card) and aggregates avoidance + steps + tokens per arm into a `Report`
(`AvoidanceOff()`/`AvoidanceOn()`), with two `Outcome`s per case. `GoldTasks()` ships 3
probes tied to **validated, executably-verifiable** traps — `exp-2868` (React 19 `useRef`),
`exp-2870` (RN `<View>` text style), `exp-0001` (FTS5 `MATCH` raw input) — each a task whose
naive answer hits the trap, with the card text and the verify key. Gate:
`internal/agenteval/agenteval_test.go` (the on-vs-off aggregation + card-injection contract).

## Progress (slice 2 — live runner + validated verifier, 2026-06-29, PR #423)

`internal/agenteval` now drives **real memory-on-vs-off numbers**:
- **`ModelRunner`** — `AgentRunner` over an OpenAI-compatible chat-completions endpoint
  (pinnable, endpoint-agnostic; the card is injected only in the memory-ON arm).
- **`BrokerVerifier`** — an *executable* avoidance check via `internal/repro`'s gVisor broker:
  per `VerifyID` it scaffolds the model's code and runs the toolchain. TS traps npm-install the
  exact `@types` in the networked prepare phase, then run `tsc` via tsconfig (TS2554/TS2769 = the
  trap bit); fts5 runs `go build`. Exit 0 = avoided.

**Instrument validation came first.** A `TestLive_VerifierDiscriminates` control (a known trap-*hit*
must score `AVOIDED=false`, a clean input `AVOIDED=true`) caught a vacuous-pass bug: npm wrote to the
sandbox user's `/nonexistent` HOME, the install failed, `tsc` no-op'd, and **everything** scored
"avoided". Fixed (`HOME`/`TMPDIR=/work`; a failed prepare now ERRORS, never silently passes). Only
after that control passes is any avoidance number trustworthy.

**First trustworthy number:** react19-useref / `qwen2.5-coder:14b` (local, off-pool) → AVOIDED in
**both** arms (off `useRef<number>(0)`, on `useRef<number|null>(null)`) — a real **null result**: a
strong coder already avoids this trap, so the card changed style (+36 tokens) not outcome. Meta-
insight: a card's *value* only shows on traps the base model genuinely fails.

**Remaining (follow-up):** (1) a faithful *runtime* fts5 check — `go build` only proves the code
compiles, not the MATCH parse trap; (2) live-validate rn-viewstyle (RN type deps); (3) a
`twiceshy agent-eval` command / `make` target wrapping the env-gated `TestLive_*`; (4) curate gold
traps a base model genuinely fails, so the on/off delta is non-null. Live runs are env-gated
(`TWICESHY_AGENTEVAL_LIVE`), skipped in CI. Runner/verifier implemented by the in-harness Sonnet
implementer (ask-cursor hung); spec + gate + the instrument-validation review by Claude.

## Notes
**Re-scoped off #0002 (2026-06-19):** the eval measures the PULL path
(`search_experience`), which IS the injection path — push (#0002) was deferred, so
the old dependency is stale. Removed `depends_on: [0002]`. Still uses a non-trivial
corpus (#0007 + the live feed).
