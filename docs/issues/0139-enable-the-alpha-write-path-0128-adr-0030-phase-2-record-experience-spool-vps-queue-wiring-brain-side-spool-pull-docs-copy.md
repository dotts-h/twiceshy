---
id: 0139
title: Enable the alpha write path (#0128 / ADR-0030 phase 2): record-experience spool, VPS queue wiring, brain-side spool pull, docs copy
status: closed
severity: high
group: 0124
depends_on: [0135, 0136]
forgejo:
links:
  adr: ADR-0030
  prs: [547, 548]
  issues: [0128, 0135, 0136]
  regression:
assets: []
---

## Summary

ADR-0030 phase 2: open the write path to alpha tenants. The hardening
blockers (ADR-0031/0032) are closed and deployed; what remains is plumbing —
there is no code switch. Gap: record_experience only RETURNS a quarantined
draft with "open it as a PR", but alpha users cannot PR the private corpus
repo, so hosted contributions are lost. report_outcome/report_issue already
spool when their queues are set, but the hosted instance wires no queues.

Scope:
1. Engine: a record-experience spool (spool.RecordDraft + EnqueueRecord/
   ReadRecord, Config.RecordQueue, `-record-queue`, `intake-records` drain)
   mirroring the report/issue queues — the spool stores the post-policy
   REQUEST (stamped author), ids allocated at drain time against the live
   corpus (the spool philosophy).
2. VPS: wire -record-queue/-report-queue/-issue-queue to volume-backed
   spool dirs.
3. Brain: one-way spool pull (like the backup pull — no LAN credential on
   the VPS) + intake drains feeding the normal quarantine → judge ladder.
4. Docs: landing docs copy announcing the write tools for alpha tokens;
   runbook section for the spool plumbing.

## Repro
1. As an alpha tenant on the hosted instance, call record_experience with a
   novel lesson.
Expected: the contribution lands in the moderation pipeline (quarantined).
Actual (before): the draft is returned to the caller with "open it as a PR"
against a repo they cannot access; nothing is captured server-side.

## Evidence

## Notes

Depends on 0135/0136 (the enablement blockers, closed). Horia's go given
2026-07-07.
