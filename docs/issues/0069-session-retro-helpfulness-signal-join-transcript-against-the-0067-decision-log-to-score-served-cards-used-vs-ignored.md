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

## Progress

- [x] **Verdict→reinforcement core (the deterministic half of acceptance 1/2).**
      `internal/retro` ships the seam + record path: `CardVerdict`, the `UsageJudge`
      interface (+ a network-free `StubUsageJudge`) that emits a per-served-card
      used/ignored verdict from a transcript, and `RecordHelpfulness`, which folds the
      *Used* verdicts into the existing usage seam through the narrow `ConfirmHelpfuler`
      (satisfied by `*index.Index.ConfirmHelpful`) — off the hot path, never influencing
      ranking (ADR-0013 §4). An *ignored* served card is an absent positive, never
      counter-evidence. Guards: `internal/retro/helpful_test.go`.
- [x] **Decision-log session-correlation key + served-set reader (acceptance 2 substrate).**
      `telemetry.Decision` now carries a salted `session` hash ([ADR-0025](../adr/ADR-0025-session-correlation-key-on-gate-decision-telemetry.md)),
      stamped on the search channel from the MCP session id (`req.GetSession().ID()`) —
      hashed like `query_hash`, raw id never persisted; a session-less request records no
      key. `telemetry.ServedInSession(path, sessionHash)` returns a session's served-id set
      (across the active log + its rotated generation), so a verdict can be cross-checked
      against what was actually served rather than trusting the transcript/model. Guards:
      `internal/telemetry/served_test.go`, `internal/server/session_decision_test.go`.
- [ ] **Remaining (tracked follow-up).** (a) the off-pool `ModelUsageJudge` prompt/edge
      that produces real verdicts (mirrors `ModelAnalyzer`) + wiring into the
      `retro-intake` drain; (b) the **join orchestration** that ties it together — hash a
      captured transcript's session id with the deployment salt, pull its served set via
      `ServedInSession`, and confirm only Used verdicts in that set (the served-set
      cross-check, now that the substrate exists); (c) acceptance 3's precision/recall
      reporter on a real-traffic sample (feeds #0005).

Issue stays **open**: this slice lands the deterministic verdict→confirm core; the model
edge, the decision-log attribution (#2), and the reporter (#3) remain.

## Notes
Split out of #0065 (whose Notes bless shipping the extraction half independently).
Soft-depends on #0067 (decision log, merged PR#269) — already in place; no hard
`depends_on` edge. Relates to ADR-0018 (the capture spine) and the #0064 epic acceptance.
