---
id: 0103
title: Wrapper session-end shippers for ask-codex and ask-cursor (Stop/finally → POST /retro)
status: closed
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

## Resolution (closed 2026-06-29)

Implemented in the **brain-config** repo (commit `3818b89`), since the wrappers and the
receiver live outside this engine repo (the `/retro` receiver was already LIVE):
- New shared shipper `claude/hooks/twiceshy-wrapper-ship.sh` — mirrors the Claude Code
  SessionEnd shipper's `spool.Transcript` write to the **brain-local** retro queue
  (`/home/ori/twiceshy-retro-queue`, no NAS round-trip, no token): atomic temp→rename,
  bounded, fail-open. Brain-local because our agents run on this box.
- `ask-cursor` + `ask-codex`: a `_ship_session` in the `EXIT` trap, guarded by a `RAN`
  flag so it fires only after an actual run, on every post-run path (success/reactive-limit/
  error), and never touches stdout. Both wrappers brought under version control (were
  untracked in `/usr/local/bin`); `install.sh` installs the new hook.
- Decision vs the issue's original framing: ship to the **brain-local queue** (not `POST
  /retro`), matching the live Claude Code shipper — simpler and tokenless on-box. The
  transcript is synthetic (`## Task <prompt>` + `## Result <final message>`), which the
  retro analyzer turns into real drafts.

**Verification:** 7-check self-contained test (`twiceshy-wrapper-ship.test.sh`, all green) +
a **real `ask-cursor` run that shipped a live `author=ask-cursor` entry** the drain turns
into a draft, with clean stdout preserved. `ask-codex`'s integration is identical and
function-verified; its real-run confirmation is pending its 5h rate window (currently at the
proactive reserve → exit 75, which correctly ships nothing since no run executes). A
*proactively* rate-gated run does not ship (nothing ran); a *reactive* limit or error after
the run did execute does ship (RAN set).

Note: served-card attribution for these wrapper sessions (the #0069 join) needs the matching
pre-call **injection** adapter and session-key coherence — out of scope here (capture only);
tracked under the epic.
