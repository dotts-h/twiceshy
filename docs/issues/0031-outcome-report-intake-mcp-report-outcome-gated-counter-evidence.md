---
id: 0031
title: Outcome-report intake — MCP report_outcome, quarantined counter-evidence (propose-only)
status: open
severity: medium
group: 0027
depends_on: []
forgejo:
links:
  adr: ADR-0013
  prs: []
  issues: [0011, 0019, 0032]
  regression:
assets: []
---

## Summary

The inbound half of the closed loop: a consuming agent that found a served lesson
**didn't work** reports it via a new MCP `report_outcome` tool —
`{record_id, outcome, failing repro/error}`. The report is stored as **quarantined
counter-evidence / a revalidation request**, never a direct mutation: it can only
*propose* re-validation work that the gate (#0032) adjudicates. "A report is
evidence, not a verdict" (ADR-0013 §3).

## Touches

`internal/server` (new MCP tool, propose-only like `record_experience`) +
`internal/ingest` (build the counter-record). Report content passes the **same
content screen** as ingest (#0011/#0019) — it's untrusted text headed for a repro.

## Acceptance

- [ ] `report_outcome` accepts `{record_id, outcome, evidence}`; validates the
  record exists; stores a quarantined counter-record / revalidation request.
- [ ] Does **not** mutate the referenced record (propose-only); does NOT write
  `validated`/`stale` directly.
- [ ] Report text is screened (secret/harmful/PII) before storage; the channel is
  **authenticated, rate-limited, and body-capped**, and re-validation work it
  triggers is **budget-capped** (#0033) so it can't DoS the broker/judge.
- [ ] A bare/non-reproducible report becomes a triage flag, not an auto-demotion.
- [ ] Test-first; `make ci` green.

## Notes

The strongest signal carries a reproducible artifact (a failing command/test);
subjective "didn't help" is a triage flag at best (ADR-0013 — signal quality).
Feeds #0032, which turns a report into a repro and gates it.
