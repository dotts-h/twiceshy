---
id: 0103
title: Wrapper session-end shippers for ask-codex and ask-cursor (Stop/finally → POST /retro)
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

Codex CLI and Cursor/Composer have **no usable `SessionEnd`** (Codex exposes only per-response
`Stop`; Cursor's hook surface is unverified — ADR-0026 §Options O1). Per O3, capture them at
the **outer wrapper** instead: `ask-codex` and `ask-cursor` ship the run transcript to
`POST /retro` from a `finally`/exit block. These two are this environment's **default
researcher and implementer**, so they generate the most currently-uncaptured sessions —
highest local value of the three adapters.

## Approach
- In `/usr/local/bin/ask-codex` and `/usr/local/bin/ask-cursor`, add a trap/`finally` that, on
  exit (success or limit/error), POSTs the captured transcript to twiceshy `/retro` with
  `{session_id, transcript, author}` (`author=ask-codex` / `ask-cursor`).
- `ask-cursor` already emits a final JSON result object (`--output-format json`); `ask-codex`
  has `-o/--output-last-message`. Capture the run's transcript/result for shipping; bound size.
- Carry a session key so the #0069 join can attribute served cards (ADR-0025). These wrappers
  are LAN-local → no-secrets concern for transcript content still applies (the `/retro` screen
  masks secrets server-side).
- Lives in the wrapper toolchain (external to this repo); the twiceshy `/retro` receiver is
  already LIVE.

## Acceptance
1. A real `ask-codex` run and a real `ask-cursor` run each ship a transcript to `/retro`.
2. `retro-intake` produces quarantined drafts attributed to `ask-codex` / `ask-cursor`.
3. Shipping fires on the limit/exit paths too (exit 75 / non-zero), not only on success.

## Notes
- Split-deliverable OK: do `ask-codex` first (named in ADR), then `ask-cursor` (same pattern).
- Child of #0101; parallelizable with 0102/0104. Establishes the wrapper-finally pattern the
  gateway floor (0104) reuses for non-agentic calls.
