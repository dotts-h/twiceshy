---
id: 0069
title: Session-retro helpfulness signal — join transcript against the #0067 decision log to score served cards used vs ignored
status: open
severity: medium
group: 0064
depends_on: []
forgejo: 297
links:
  adr: docs/adr/ADR-0018-session-retro-capture.md
  prs: []
  issues: [0064, 0065, 0067]
  regression:
assets: []
---

## Summary
The session-retro capture spine (#0065, [ADR-0018](../adr/ADR-0018-session-retro-capture.md))
ships the **trap-extraction** half: a `SessionEnd` hook → `/retro` → off-pool
analyzer → quarantined drafts. This issue is the **deferred measurement half**: close
the loop on *"did the injected/served card actually help?"* by joining a captured
session transcript against #0067's per-query gate-decision log to score which
served/pushed cards were **used vs ignored**, feeding that into the eval direction
(#0005) and `provenance.usage.confirmed_helpful`.

## Motivation
#0064(c): the measurement loop is open — usage counters are logged only in aggregate,
so helpfulness is unmeasurable on real traffic. #0067 now records, per query, which
cards were served (channel, `query_hash`, served ids). A retro analyzer that already
holds the full transcript can determine whether a served card's lesson was actually
applied — closing the loop with no new instrumentation.

## Approach
- The retro analyzer (`internal/retro`), given a transcript, additionally emits a
  per-served-card used/ignored judgement (reuse the off-pool analysis pass added for
  #0065).
- Join on session/query against the #0067 telemetry decision log
  (`internal/telemetry`) to attribute served cards to the session.
- Feed the confirmed-helpful signal through the existing `confirm_helpful` / usage seam
  (`index.ConfirmHelpful`) — off the hot path, never influencing ranking (ADR-0013 §4).
- Surface precision/recall on a sample of **real** traffic for the eval (#0005), not
  only the synthetic positive/negative sets.

## Acceptance
- [ ] A captured session yields, per served/pushed card, a used-vs-ignored verdict.
- [ ] The verdict is attributed via the #0067 decision log and recorded through the
      existing usage seam (no new ranking influence).
- [ ] Precision/recall reported on a real-traffic sample (feeds #0005).

## Notes
Split out of #0065 (whose Notes bless shipping the extraction half independently).
Soft-depends on #0067 (decision log, merged PR#269) — already in place; no hard
`depends_on` edge. Relates to ADR-0018 (the capture spine) and the #0064 epic acceptance.
