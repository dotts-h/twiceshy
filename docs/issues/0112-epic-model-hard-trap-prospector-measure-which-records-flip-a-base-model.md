---
id: 0112
title: Epic: Model-hard trap prospector — measure which records flip a base model
status: open
severity: high
group: 
depends_on: []
forgejo:
links:
  adr: docs/adr/ADR-0029-model-hard-trap-prospector.md
  prs: []
  issues: [0005, 0106]
  regression:
assets: []
---

## Summary
The validated corpus has no per-record measured-value signal. The one live gold
case for #0005's on/off eval is a NULL result — qwen2.5-coder:14b avoids the
React-19 `useRef` trap unaided, on/off arms identical (2026-07-01 measurement,
post ADR-0028/#0106's push-precision fix) — and lifetime usage counters
(12,304 pushed / 1 confirmed_helpful) proved unmeasured value is
indistinguishable from no value. This epic builds an autonomous measurement
loop that finds which validated records a base model actually fails without
help — **model-hard** records — by reusing #0005's own components instead of
hand-curating gold traps by guess: `agenteval.ModelRunner`
(`internal/agenteval/runner.go:105`), `BrokerVerifier`
(`internal/agenteval/verifier.go:56`), and the gVisor-backed `repro.Broker`.
Loop: record → drafted task (an LLM seam that must not leak the escape) →
base-model OFF-arm run → executable verdict; failures get an ON-arm run with
the card injected, and the avoidance delta IS the record's measured value.
Outputs (report + gold cases) stay in the engine repo; corpus records are
never mutated.

## Children
- 0113 — prospector core loop (TaskDrafter seam, OFF-arm run, verify classes,
  leak guard, `twiceshy prospect` command)
- 0114 — gold emission: model-hard failures become #0005 gold cases with the
  measured ON-arm delta

## Evidence
- #0005's live gold set has exactly one case and it is a null result (shared
  context, 2026-07-01).
- Epic #0106's lifetime usage counters: Σpushed 12,304, Σconfirmed_helpful 1 —
  unmeasured value is indistinguishable from no value.
- `internal/agenteval/agenteval.go:18` (`TaskCase`) and its `AgentRunner`/
  `Verifier` interfaces already exist from #0005 slice 2 — the prospector is
  new orchestration over existing infrastructure, not a new harness.

## Acceptance (epic closes when)
- [ ] The prospector runs env-gated against the live corpus + local models.
- [ ] The report lists scanned / drafted / skipped (with reasons) / model-hard
      counts.
- [ ] At least one genuinely model-hard record is found, or an honest null is
      reported (mirroring #0005's own "empty is an answer" discipline).
- [ ] #0005's gold set grows only from measured failures, never from a guess.

## Notes
Wave 2 of the 2026-07-01 planning session (orchestrator: Claude), following
ADR-0028/epic #0106's precision fix — serving is only worth measuring once the
served subset is agent-actionable. ADR-0029 records this epic's decision.
