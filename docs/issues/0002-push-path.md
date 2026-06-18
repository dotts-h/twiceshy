---
id: 0002
title: Push path — hook → trap cards via additionalContext
status: done
severity: high
group: 0008
depends_on: [0004]
forgejo:
links:
  adr: docs/adr/ADR-0001-architecture.md
  prs: [49, 50]
  issues: [0005]
  regression:
assets: []
---

## Summary
Deterministic injection at decision time (Phase 2): a Claude Code
`UserPromptSubmit`/`PreToolUse` hook calls a plain HTTP endpoint that returns
**1–3 trap cards** via `additionalContext`, on **high-confidence + validated**
matches only. This is the channel that makes twiceshy fire "in each
implementation" instead of waiting to be asked (ADR-0001 §5). The hot path is
embedding-free by rule (fingerprint + BM25 only; ADR-0001 §4).

## Children (file when broken down)
- HTTP push endpoint (plain, not MCP) returning trap cards above the relevance floor.
- Trap-card renderer: title, applicability conditions, the trap, the escape; k≤3.
- Hook script(s) for `UserPromptSubmit` / `PreToolUse` + install docs.
- Ambient **index channel**: generated skill / `AGENTS.md` one-liner so the model
  knows the store exists (CONTEXT.md: never rely on the model asking).

## Acceptance
- [ ] Quarantined records NEVER pushed; only `validated` reach the push channel.
- [ ] Near-miss defenses hold: relevance floor, hard cap k≤3, stack-fingerprint
      filter, applicability printed in the card.
- [ ] Injecting nothing on a weak match is verified behavior, not a bug.

## Notes
Depends on #0004 (D3) — the push channel needs *validated* records to have
anything to inject.
