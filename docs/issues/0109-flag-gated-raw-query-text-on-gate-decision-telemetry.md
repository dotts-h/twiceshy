---
id: 0109
title: Flag-gated raw query text on gate-decision telemetry
status: closed
severity: medium
group: 0106
depends_on: []
forgejo: 475
links:
  adr:
  prs: [481]
  issues: [0106, 0067]
  regression:
assets: []
---

## Summary
Gate-decision telemetry (`telemetry.Decision`, `internal/telemetry/decision.go:37`,
#0067) logs only `QueryHash` — a salted hash of the query, never the raw text
(`recordPushDecision`, `internal/server/push.go:160`, is explicit that "the raw
query is hashed, never stored"). That is the right default, but it makes
precision failures unauditable from telemetry alone: tonight's diagnosis of the
epic-#0106 collapse required the **live prompt in hand** (pulled from a session
transcript, not the decision log) to see which tokens fired the gate. Add a
serve flag `-telemetry-query-text` (default OFF) that, when set, has
`telemetry.Decision` carry the query text too — truncated to 256 bytes — so a
single-tenant deployment can opt into debuggable gate decisions without changing
the default hash-only posture.

## Repro
1. Reproduce a push-gate false-positive against the live server and try to
   diagnose it from `TWICESHY_TELEMETRY_LOG` alone.
Expected: the decision log record identifies which tokens/text triggered the
serve, closing the loop without needing a side-channel transcript pull.
Actual: only `query_hash` is present — useless for a human diagnosing *why* a
specific prompt served a card; the live specimen this epic diagnosed had to be
retrieved from the session transcript, not telemetry.

## Evidence
- `internal/telemetry/decision.go:37-46`: `Decision.QueryHash` only; no query-text
  field exists today.
- `internal/server/push.go:135` / `:160-181` (`recordPushDecision`): "the raw
  query is hashed, never stored" — this issue keeps that as the compiled-in
  default and adds an explicit, off-by-default opt-out.
- `cmd/twiceshy/main.go:390` (`-telemetry-log`) is the existing precedent for an
  opt-in serve flag gating this subsystem.

## Acceptance
- Flag off (default): `telemetry.Decision` has no query-text field populated —
  byte-identical wire behavior to today (`omitempty` on the new field, or the
  field absent entirely from the JSON when unset).
- Flag on: `Decision` carries the query text, truncated to 256 bytes, alongside
  the existing hash (hash stays **always** present regardless of the flag).
- Documented in `docs/DEPLOY.md`'s flag table if one exists — as of this writing
  `docs/DEPLOY.md` has no consolidated flag table (flags are documented inline
  near their usage, e.g. `-hold-cooldown` at `docs/DEPLOY.md:185`); if a table
  exists by the time this lands, add the row there, otherwise document
  `-telemetry-query-text` inline the same way.

## Notes
Single-tenant deployments (the only deployment shape today, per ADR-0021's still-
proposed multi-tenant split) are the intended audience for turning this on: the
privacy cost of raw query text in the decision log is borne entirely by the
deployment's own operator. Complements #0067 (the log this extends) and is
scoped by the epic's evidence: this diagnosis session is the concrete case for
why hash-only telemetry was too little.
