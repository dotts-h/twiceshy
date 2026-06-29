---
id: 0102
title: Gemini CLI SessionEnd shipper — POST /retro on session end (mirror the Claude Code hook)
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

Gemini CLI exposes a `SessionEnd` lifecycle hook (like Claude Code). Build the shipper that,
on session end, ships a bounded session transcript to twiceshy's `POST /retro` so Gemini CLI
sessions feed the retro capture spine — the same path the Claude Code hook already uses.

## Approach
- Mirror the live Claude Code shipper `hooks/session-retro.sh` / the brain wrapper
  `twiceshy-retro-ship.sh`: read the session transcript, bound its size, POST to `/retro`
  with `{session_id, transcript, author}` (carry the #0069 session key / served-card context
  per ADR-0025 so the helpfulness join can attribute).
- Register it in the Gemini CLI hook config (geminicli.com/docs/hooks). Keep it
  latency-bounded — `SessionEnd` runs at teardown but should not hang the CLI.
- The twiceshy receiver (`internal/server/retro.go`) is already LIVE; no engine change
  expected unless the payload shape needs a tweak.

## Acceptance
1. A Gemini CLI session that solves a novel trap ships its transcript to `/retro`.
2. The next `retro-intake` drain produces a quarantined draft attributed to a `gemini-cli`
   author/source.
3. If the session was served cards, the #0069 join attributes used/ignored against them.

## Notes
- Transcript is untrusted DATA (already framed as such in `internal/retro`).
- Child of #0101 (ADR-0026 O3); parallelizable with 0103/0104.

## Resolution (closed 2026-06-29)

Implemented in the **brain-config** repo (commit `4978617`), since the Gemini surfaces and
the receiver live outside this engine repo (no engine change was needed).

**Premise correction (verify-on-real-signal).** The charter assumed a single "Gemini CLI"
with a native `SessionEnd` hook. Ground truth in this environment: there was **no `gemini`
CLI installed**, and the brain's working Gemini surface is **Antigravity (`agy` / `ask-agy`)**,
whose `agy` CLI exposes **no `SessionEnd` (or any) lifecycle hook** (`agy --help` verified).
So the Gemini surface split into two adapters:

- **`ask-agy` + `ask-gemini` → wrapper-finally** (the #0103 pattern, since `agy` has no native
  hook): `ask-agy`'s terminal `exec agy …` was replaced with a captured run + an `EXIT`-trap
  `_ship_session` (an `exec` replaces the shell, so a trap could never fire); `ask-gemini` ships
  the prompt+answer on its success path. Authors `ask-agy` / `ask-gemini`. `ask-agy` was also
  brought under version control (it was untracked in `/usr/local/bin`).
- **`gemini-cli` (`@google/gemini-cli`, installed) → native hook** (the charter's literal
  approach, now realized): it *does* expose a native, **Claude-Code-compatible** `SessionEnd`
  hook (snake_case `{session_id, transcript_path}`; it even ships a `hooks migrate` from Claude
  Code). New thin adapter `claude/hooks/twiceshy-gemini-ship.sh` renders the transcript and
  delegates the spool write to `twiceshy-wrapper-ship.sh` (`author=gemini-cli`,
  `reason=session-end`). Wired idempotently into `~/.gemini/settings.json` by `install.sh`.
- `twiceshy-wrapper-ship.sh` gained a backward-compatible `--reason` flag so the spool-write
  logic stays in one tested place (native hooks record `session-end`, wrappers `wrapper-exit`).

**Decision vs the issue's original framing:** ship to the **brain-local retro queue** (not
`POST /retro`), matching the live Claude Code shipper and #0103 — simpler, tokenless on-box.

**Verification (real signal):** a 15-check self-contained test (`twiceshy-wrapper-ship.test.sh`,
all green, including a full stubbed-run guard for the `ask-agy` exec→capture restructure) +
**real `ask-agy`, `ask-gemini`, AND `gemini-cli` sessions that each produced a live spool entry**
with the right author/reason and rendered transcript. A synthetic-trap Gemini transcript drains
to a **quarantined draft** (`retro-intake -dry-run` → `would create exp-…`), confirming the
capture→draft chain for the Gemini surface (acceptance 1–2). This satisfies the epic's
"≥2 non-Claude-Code surfaces" goal (ask-agy, ask-gemini, gemini-cli + #0103's ask-cursor/ask-codex).

Acceptance 3 (served-card #0069 attribution) needs the matching pre-call **injection** adapter
and session-key coherence — out of scope here (capture only); tracked under epic #0101. Second
child of the epic done; #0104 (the non-agentic gateway floor) remains.
