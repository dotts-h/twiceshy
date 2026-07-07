---
id: 0136
title: Unified alpha write-path policy seam — forced origin stamping, caps and secret posture for report_outcome/report_issue, quota for confirm_helpful, operator-only /retro
status: open
severity: high
group: 0124
depends_on: [0135]
forgejo:
links:
  adr: ADR-0031
  prs: []
  issues: [0128, 0135]
  regression:
assets: []
---

## Summary

The #0128 alpha hardening (forced `alpha:<token_id>` origin stamping, alpha
size caps, forced redaction, fail-closed secret rejection, contribution
quota) is applied in full only to `record_experience`. The 2026-07-07
pre-enablement architecture review found the policy scattered as per-handler
`if alpha {...}` blocks, with every other write surface getting a subset:

- `report_outcome` / `report_issue`: quota only — `args.Author` flows
  verbatim into `provenance.source.author` (`internal/server/report.go:87`,
  `issue.go:94,103`). Dispute counter-records feed the adapt/demote loop,
  which counts independent authors as corroboration: one hostile token can
  fabricate N "independent" disputes against a validated record, and can
  spoof importer/trusted origins (#0118's trust key).
- `confirm_helpful`: no contribution bound — reinforcement-counter inflation
  at rate-limit speed.
- `POST /retro`: no alpha posture — any tok_ token spools 256 KiB
  transcripts per call into the off-pool analyzer queue.

## Fix (ADR-0031)

Declare the policy once (`alpha_policy.go`: per-tool quota table + shared
stamping/caps/secret-posture helpers), apply it uniformly (report/issue get
the full posture; confirm_helpful gets a 50/day quota; /retro answers 403
for tok_ tenants), and add a completeness test that fails when a write
surface is missing from the declaration.

## Repro
1. As an alpha tok_ tenant, call `report_outcome` with
   `author: "osv-importer"` against any validated record.
Expected: the counter-record's `provenance.source.author` is
`alpha:<token_id>`; the caller string survives only as a display note.
Actual (before fix): `provenance.source.author: osv-importer` — a spoofed
trusted origin.

## Evidence

- `internal/server/report.go:87` — `meta := ingest.Meta{..., Author: args.Author, ...}`.
- `internal/ingest/report.go:92` — `Source{Author: m.Author}` into provenance.
- Architecture review 2026-07-07 (session), finding A; per-handler survey table.

## Notes

Depends on 0135 (ADR-0032) — confirm_helpful's quota charges the new atomic
debit. Together they gate #0128 write-path enablement for alpha tenants.
