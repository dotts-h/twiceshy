---
id: 0102
title: Gemini CLI SessionEnd shipper — POST /retro on session end (mirror the Claude Code hook)
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
