---
id: 0019
title: Extend ingestion screen to repro-script content + execution trust boundary
status: open
severity: high
group: 0015
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

Must-have C (ADR-0011 §4): extend `internal/screen` to screen **repro-script
content** (the executable text), not just record prose — and enforce the **trust
boundary**: only PR-reviewed + ingest-screened repros may be executed by the
broker. A repro legitimately contains code, so calibrate rules for *scripts*
(don't false-positive on every test) while still catching exfil / secret /
destructive payloads.

## Touches

`internal/screen/screen.go` (`Scan(texts...)`), wired into ingest + the broker gate.

## Acceptance

- [ ] Screen accepts repro-script content; flags secrets / exfil / destructive ops
- [ ] Broker refuses to run an unscreened / flagged repro (trust boundary enforced)
- [ ] Test-first; calibrated against existing repro fixtures (no false reject)
