---
id: 0104
title: Gateway floor for non-agentic executors — code-exec injects pre-call and emits a session-end record on close
status: open
severity: medium
group: 0101
depends_on: []
forgejo:
links:
  adr: docs/adr/ADR-0026-runtime-enforcement-of-experience-adoption.md
  prs: []
  issues: [0101]
  regression:
assets: []
---

## Summary

Non-agentic executors call models directly (DeepSeek/GLM/Qwen via OpenRouter/Ollama) and have
**no session lifecycle at all** (ADR-0026 §Options). Per O3, give them a single **gateway
floor**: the point that constructs the model request injects matched cards **pre-call** and
emits a **synthetic session-end record** on close — collapsing every non-agentic surface into
one enforcement point. `code-exec` already centralizes these calls and is the natural host.

## Approach
- At the `code-exec` call boundary: pre-call, query twiceshy push/`search_experience` for the
  task and prepend matched cards (latency-bounded; served text stays untrusted DATA).
- On close, emit a session-end record to `/retro` (or a dedicated endpoint). The payload is
  **not a rich transcript** — it's a synthetic one: the injected card ids + the prompt/response
  exchange, enough for the #0069 join to label served cards used/ignored (ADR-0025 key).
- **Open design question (resolve in this issue):** reuse `POST /retro` with a synthetic
  transcript wrapper, OR add a thin endpoint for the gateway-floor shape. Prefer reuse if the
  analyzer can extract verdicts from the synthetic transcript; otherwise spec the new shape.

## Acceptance
1. A non-agentic `code-exec` call injects ≥1 matched card pre-call when one is relevant.
2. On close it emits a record that the #0069 join can attribute (served → used/ignored).
3. No model self-report is involved — feedback is observed from the exchange.

## Notes
- Highest effort + most spec-dependent of the three (the synthetic session-end shape).
- Lowest priority adapter per ADR-0026 (behind the agentic surfaces) — but the only coverage
  for direct model calls. Child of #0101; reuses the wrapper-finally pattern from 0103.
