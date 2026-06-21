---
id: 0067
title: Per-query gate-decision telemetry
status: closed
severity: medium
group: 0064
depends_on: []
forgejo: 253
links:
  adr: docs/adr/ADR-0015-push-discriminative-term-gate.md
  prs: []
  issues: [0064, 0005, 0065]
  regression:
assets: []
---

## Summary
Today only **aggregate** counts are logged (`hits`, `count`) for
`search_experience` and `/push`. There is **no per-query record** of *why* a card
was or wasn't served: no candidate ids, no scores, no relevance-floor decision.
Consequence: **precision/recall cannot be measured on real traffic** — the only
eval is 16 synthetic negatives + 6 positives, and we could not even answer "did
the gate leak?" without manually curl-probing the live endpoint.

## Approach
- Log, per query, a structured decision record:
  `{ query-hash/redacted, candidate_ids, scores, floor_decision, served_ids }`
  for both `/push` and `search_experience`.
- This is the **substrate** that lets:
  - eval #0005 compute precision/recall on **sampled real traffic**, with Wilson
    confidence intervals (reported for honesty; not a gate at tens-of-records);
  - the retro analyzer (#0065) compute a real **helpfulness** signal
    (served → used vs ignored).

## Privacy / invariants
- **Hash or redact** the raw query (it may contain sensitive prompt text);
  store enough to reconstruct the decision, not the user's content verbatim.
- LAN-only; **retention cap** (rolling window). Off the hot path (async write,
  like the existing usage recorder).
- Telemetry **must not** influence ranking (ADR-0013 §4 keeps usage out of
  ranking) — this is for measurement only.

## Open questions
- Where to persist: extend the SQLite `usage` store with a `decisions` table, or
  a separate append-only log? Retention + size budget.
- Redaction strategy that still lets us label relevance offline (a salted hash
  loses the text needed for labeling — may need a short-TTL plaintext window on
  LAN before redaction).

## Decision (resolves the open questions above)
- **Persistence: an append-only, rotating JSONL log** (`internal/telemetry`), not a
  SQLite `decisions` table. It is structurally separate from the ranking index — so it
  *cannot* influence ranking (ADR-0013 §4) — has trivial retention (rotate at
  `telemetryMaxBytes`, keep one prior generation → bounded to ~2x), appends cheaply off
  the hot path, and streams for the offline eval (#0005).
- **Redaction: retrieval tokens + a salted query hash; the raw query is never persisted.**
  Push records the discriminative gate tokens (already stopword/ecosystem-filtered) +
  the fingerprint-bypass flag; search records served ids+scores. The salt defaults to the
  bearer token (a per-deployment secret) so a hash can't be dictionary-attacked. Tokens +
  served ids are enough to label relevance offline — no plaintext window needed.
- **Capture seam: the server handlers**, via `Index.RetrievePushTraced` (the gate decision
  exposed with no extra hot-path work; `RetrievePush` is now a thin wrapper). Writes are
  async + best-effort (a single writer, drop-on-overload), so telemetry never slows serving.
- **Enablement: opt-in** via `serve -telemetry-log <path>`. Enabling it on the live serve
  unit is the ops step that starts collection; #0005 consumes the log for real-traffic
  precision/recall.

## Notes
Foundational child of epic #0064 — suggested build-first. Directly unblocks the
"measure" third of the epic and the real-traffic reframe of eval #0005.
