---
id: 0130
title: Contribution data-license terms + privacy policy for the hosted alpha
status: closed
severity: medium
group: 0124
depends_on: []
forgejo: 531
links:
  adr:
  prs: [519]
  issues: []
  regression:
assets: []
---

## Summary

Write the contribution data-license terms + privacy policy for the hosted
alpha: contributors grant a license to their submitted records (the
CLA-for-data analogue of ADR-0002), what telemetry is collected per token, PII
handling and deletion, and the AGPL relationship (self-hosting always free).
Accepted at signup (#0127 checkbox); versioned in-repo like ADRs.

## Notes

Not legal-perfect for alpha, but explicit: people are submitting engineering
failure data — say plainly what we do with it, that secrets must not be
submitted, and that submissions become part of a public validated corpus.

## Close-out (2026-07-06, PR #519)

Shipped: alpha terms & data policy v1 as `web/landing/terms.html`, linked
from both signup forms. Not legal-perfect — revisit the wording before the
write path opens publicly (see #0128's phase-2 gate).
