---
id: 0101
title: 'Epic: Fleet-wide enforcement adapters (ADR-0026 O3) — capture & inject across Gemini CLI, ask-codex/ask-cursor wrappers, and the non-agentic gateway floor'
status: closed
severity: medium
group:
depends_on: []
forgejo: 468
links:
  adr: docs/adr/ADR-0026-runtime-enforcement-of-experience-adoption.md
  prs: []
  issues: [0102, 0103, 0104, 0064, 0005]
  regression:
assets: []
---

## Summary

[ADR-0026](../adr/ADR-0026-runtime-enforcement-of-experience-adoption.md) (O3 hybrid,
accepted 2026-06-29) decided that experience **injection** and **feedback** are properties
of the *runtime*, never requests to the model — "anything left to the model's discretion
does not happen." Only the **Claude Code** surface is instrumented today
(`SessionEnd` → POST /retro, LIVE 2026-06-24). This epic builds the remaining O3 adapters so
the rest of the heterogeneous fleet (Gemini CLI, the `ask-codex`/`ask-cursor` wrapper
runners, and non-agentic executors) also ship session-end transcripts and inject cards —
extending the served→used measurement chain (now functional after #0099/#0100) across the
whole fleet, which unblocks the prove-or-kill eval (#0005).

The twiceshy receiver already exists: `POST /retro` (`internal/server/retro.go`) screens +
spools transcripts, and `retro-intake` drains them. The adapter work is per-surface capture
(and pre-call injection), mostly in the runner/wrapper layer.

## Children

| # | adapter | surface | effort | note |
|---|---------|---------|--------|------|
| 0102 | Gemini CLI `SessionEnd` shipper | native hook | medium | Gemini exposes `SessionEnd` like Claude Code — mirror `twiceshy-retro-ship.sh` |
| 0103 | `ask-codex` + `ask-cursor` wrapper shippers | wrapper `finally` | medium | Codex/Composer have no usable `SessionEnd`; ship on wrapper exit. Highest local value (these are this env's default researcher + implementer) |
| 0104 | Gateway floor (non-agentic) | `code-exec` | medium-high | Direct DeepSeek/GLM/Qwen calls: inject pre-call + emit a synthetic session-end on close |

These are independent (no hard ordering) now that the measurement chain works; build by value.

## Acceptance
- [x] A Gemini CLI session that solves a trap ships a transcript to `/retro` and yields a
      quarantined draft (0102). *(Closed: agy has no native hook → wrapper-finally for ask-agy/
      ask-gemini + a native hook for the now-installed gemini-cli; ships to the brain-local queue.)*
- [x] An `ask-codex` and an `ask-cursor` run ship their transcript on wrapper exit (0103).
- [x] A non-agentic `code-exec` call injects matched cards and emits a session-end record
      attributable in the #0069 join (0104). *(Injection pre-existed; added the attributable
      session-end via a minted session threaded through `/push` + the spool entry.)*
- [ ] Served→used helpfulness is reported on real traffic from ≥2 non-Claude-Code surfaces
      (feeds #0005). *(Adapters now in place across the fleet → the measurement chain is enabled;
      empirically PROVING this on real traffic is the prove-or-kill eval #0005, which this epic
      unblocks.)*

> **All three adapter children (0102, 0103, 0104) are CLOSED** as of 2026-06-29 — the capture
> spine spans Claude Code + ask-codex/ask-cursor + the Gemini surfaces + non-agentic code-exec.
> The epic's build work is complete; the only open item is the emergent 4th acceptance, which is
> #0005's domain.

## Resolution (closed 2026-06-29)

All three O3 enforcement adapters are built, verified, and merged — the fleet-wide capture spine
is complete:
- **0102** — Gemini surface: wrapper-finally shippers for `ask-agy`/`ask-gemini` (agy has no
  native hook) **+** a native SessionEnd hook for the now-installed `@google/gemini-cli`.
- **0103** — `ask-codex` + `ask-cursor` wrapper-exit shippers (this env's default researcher +
  implementer).
- **0104** — non-agentic `code-exec` gateway floor: pre-call injection (pre-existing) + an
  attributable synthetic session-end record (minted session threaded through `/push` + the spool).

A key cross-cutting finding: **no engine change was needed for any of them** — the `/retro`
receiver, the brain-local spool/`retro-intake` drain, and the #0069 served→used join
(`/push` `session` → salted-hash decision log → hashed-`session_id` join, ADR-0025) were all
already live. Every adapter ships to the **brain-local retro queue** (tokenless on-box), the
pattern established in 0103.

The 4th acceptance — served→used helpfulness *reported on real traffic from ≥2 non-Claude-Code
surfaces* — is now **enabled** (the adapters provide the traffic) but its empirical proof is the
**prove-or-kill eval #0005**, which this epic exists to unblock. Closing per owner confirmation
(2026-06-29); the served→used reporting milestone is carried by #0005.

## Notes
- Receiver is built; the per-surface transcript shape must satisfy `internal/server/retro.go`'s
  screen + the `{session_id, transcript, author}` spool payload, and carry the served-card ids
  / session key so the #0069 join can attribute (ADR-0025 key).
- Prompt-injection surface: auto-injected card text is untrusted (ADR-0026 §risks) — the
  `internal/retro` transcript-as-DATA framing already mitigates on the analyze side; pre-call
  injection adapters must not let served text become instructions.
- Hot-path budget: pre-call inject + `UserPromptSubmit`-class hooks must stay latency-bounded.
- Continues epic 0064 (the feedback loop) and ADR-0026; unblocks #0005.
