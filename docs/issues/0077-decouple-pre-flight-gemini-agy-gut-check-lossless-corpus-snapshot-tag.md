---
id: 0077
title: Decouple pre-flight: gemini+agy gut-check + lossless corpus snapshot tag
status: open
severity: medium
group: 0076
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
Pre-flight gate (ADR-0021 phase 0): run the OWED gemini+agy frontier gut-check on the decouple plan (endpoints were down at ADR authoring), and tag origin/main:experience as a lossless snapshot so the move is provably lossless. Gates the cut-over.
