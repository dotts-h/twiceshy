---
id: 0104
title: Gateway floor for non-agentic executors — code-exec injects pre-call and emits a session-end record on close
status: closed
severity: medium
group: 0101
depends_on: []
forgejo: 471
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

## Resolution (closed 2026-06-29)

Implemented in the **brain-config** repo (commit `6c56248`). **No engine change was needed** —
the entire served→used chain was already live: `/push` accepts a `session` field and logs the
served-card decision under its salted hash (`internal/server/push.go:30,170`), `/retro` accepts
`session` (`internal/server/retro.go:23`), and `retro-intake` joins the transcript against the
gate-decision log on the **hashed `session_id`** (`cmd/twiceshy/retro.go:82`), with
`RecordHelpfulnessAttributed` enforcing the served-set (`internal/retro/helpful.go:105-119`).

**Acceptance #1 (pre-call injection) was already satisfied** by the existing `code-exec`: it
queries `/push` and injects matched validated cards into the system message (ADR-0015 floor →
0 tokens unless a card genuinely matches). This issue added the **attributable session-end** half:

- **Mint a session id per `code-exec` call**, pass it as `session` to `/push` (so the served-card
  decision is logged under its hash), and stamp the **same raw id** on the synthetic session-end
  record. Same raw id on both ends is exactly what the #0069 join needs — the salt is applied
  **server/drain-side** (ADR-0025), so `code-exec` does no salt handling (this also sidesteps the
  #0098 salt-divergence trap: serve and the brain drain already share the `TWICESHY_TOKEN` salt).
- **On a real model response** (`RAN=1`), an `EXIT`-trap ships a synthetic transcript (prompt +
  injected card ids + response) to the **brain-local retro queue** via `twiceshy-wrapper-ship.sh
  --author code-exec --reason session-end --session <id>`, reusing the #0102/#0103 shipper and the
  #0102 `--reason` flag.

**Open design question resolved:** reuse the existing pipe (the brain-local spool that
`retro-intake` already drains), **not** a new endpoint — consistent with #0102/#0103 and zero
engine change. Acceptance #3 holds by construction: feedback is observed from the exchange
(injected ids + response), never asked of the model.

**Verification (real signal):** 17-check test (`twiceshy-wrapper-ship.test.sh`, all green),
including a full stubbed-run that asserts the **attribution invariant** — the *same* minted session
on the `/push` call and the session-end record, with served ids captured. Pre-call injection fired
**live** (`[code-exec] twiceshy: injected 3 trap card(s)` via a session-stamped `/push`) across
three real runs; the **deployed** `code-exec` + deployed shipper produce a correct session-end
record with the attribution invariant holding. A fully-live *successful* model run was blocked only
by transient NVIDIA NIM unavailability (empty/timeout on every attempt) — environmental, not a code
issue, mirroring #0103's ask-codex real-run being gated by its rate window.

Third and final child of epic #0101. The fleet-wide capture spine now spans Claude Code, the
ask-codex/ask-cursor implementer/researcher wrappers (#0103), the Gemini surfaces (#0102), and
the non-agentic `code-exec` floor (#0104) — extending the served→used measurement chain across
the whole fleet, which unblocks the prove-or-kill eval (#0005).
