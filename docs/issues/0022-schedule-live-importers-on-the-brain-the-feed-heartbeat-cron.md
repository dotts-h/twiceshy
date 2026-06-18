---
id: 0022
title: Schedule live importers on the brain — the feed heartbeat (cron)
status: closed
severity: medium
group: 0015
depends_on: [0021]
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

Make the feed run **on its own** (ADR-0011): schedule the live importer(s) on the
brain (cron / systemd timer) so the corpus grows without hand-authoring — the
"data pouring in" heartbeat. Quarantined output is committed/PR'd per the trust
boundary (never a silent direct write to `validated`).

## Touches

brain scheduler (cron / systemd timer) invoking the importer.

## Acceptance

- [ ] Importer runs on a schedule; new quarantined records appear over time
- [ ] Idempotent + bounded (no runaway); observable (logs / notify)
- [ ] Depends on 0021 (a live importer to schedule)
