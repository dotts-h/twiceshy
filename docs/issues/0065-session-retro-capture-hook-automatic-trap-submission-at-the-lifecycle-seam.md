---
id: 0065
title: Session-retro capture hook — automatic trap submission at the lifecycle seam
status: closed
severity: high
group: 0064
depends_on: []
forgejo: 251
links:
  adr: docs/adr/ADR-0018-session-retro-capture.md
  prs: []
  issues: [0064, 0067, 0069]
  regression:
assets: []
---

## Summary
Agents reliably **fail to write back** the traps they solve. Pull is
self-targeting — being stuck is the trigger — but submission has no intrinsic
trigger and zero local payoff at the moment it would happen, so it never fires.
This issue adds the **missing trigger**: capture the retro automatically at the
session lifecycle seam, so the agent never has to *remember* to submit.

## Motivation
Observed directly: a real session found multiple traps and never once considered
submitting them. The corpus therefore grows only via the automated OSV importer
(homogeneous advisories) and manual dogfooding — it does **not** capture the
organic traps agents actually hit. That is a strategic risk to corpus value, not
a cosmetic gap.

A naive "remember to submit!" nudge is the wrong fix: indiscriminate hooks
become noise and get disabled — exactly why the per-prompt **push** hook was
deferred. And hooks inject context; they **cannot force a tool call**. So the
design must not depend on agent volition at all.

## Approach
- A Claude Code **`Stop` / `SessionEnd` hook** ships the session transcript (or a
  bounded summary) to a new twiceshy **retro endpoint** (LAN-only, bearer auth).
- The existing **off-pool judge stack** (gpt-oss / sonnet / gemini) analyzes it
  to: (1) extract candidate traps and feed them into the **existing
  `record_experience` quarantine → PR → validation ladder** (no new write path);
  (2) record which served/pushed cards were **used vs ignored** (the helpfulness
  signal — overlaps #0067's decision log).
- Server-side automation sidesteps the "can't force a tool call" problem: the
  capture does not depend on the agent doing anything.

## Security / invariants
- LAN-only; **no secret exfiltration** (run the content screen before anything
  leaves, and again at intake).
- The analyzer must treat the transcript as **DATA, not instructions** (reuse the
  `--- BEGIN/END EXPERIENCE DATA ---` envelope discipline) — an LLM analyzer is
  itself prompt-injectable.
- Everything extracted is **quarantined**; it reaches no other agent until it
  clears the validation ladder. Inherits auth + rate-limit + content-screen.

## Open questions
- Retro payload shape: full transcript vs. agent-authored summary vs. tool-call
  trace only? (Privacy + token cost trade-off.)
- Precision: auto-extraction will be noisy; the quarantine + judge gate must keep
  it from flooding the PR queue (same lesson as exp-0622 push precision). Low
  homelab volume makes a heavy per-session off-pool pass affordable.
- Which hook event — `Stop` (per response) is too frequent; `SessionEnd` is the
  natural seam. Confirm against Claude Code hook semantics.

## Notes
Headline child of epic #0064. Soft-depends on #0067 for the used-vs-ignored
signal, but the trap-extraction half can ship independently.

## Resolution
Shipped the **trap-extraction capture spine** ([ADR-0018](../adr/ADR-0018-session-retro-capture.md)):
- A `SessionEnd` hook (`hooks/session-retro.sh`, fail-open) ships a bounded,
  locally-screened transcript to a new **raw `POST /retro`** endpoint
  (`internal/server`), which screens at the edge (secrets refused, never spooled)
  and spools it (`internal/spool`).
- The **`twiceshy retro-intake`** driver drains the spool through an off-pool
  `retro.Analyzer` (`internal/retro`, stubbed in tests), feeding extracted traps
  through the existing `ingest.Prepare` → quarantine → PR ladder — no new write
  path, no promotion (the analyzer drafts only).
- A `twiceshy screen` subcommand exposes the tested content screen for the hook's
  client-side pass.

Headline acceptance met: a session that solves a novel trap becomes a quarantined
draft **without the agent calling `record_experience`**. The **used-vs-ignored
helpfulness signal** (the measurement half) is split out to **#0069**.
