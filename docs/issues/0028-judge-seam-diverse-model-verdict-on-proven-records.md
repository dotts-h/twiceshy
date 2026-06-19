---
id: 0028
title: Judge seam — diverse-model verdict on a proven record (meaning/scope/license/poison)
status: closed
severity: high
group: 0027
depends_on: []
forgejo:
links:
  adr: ADR-0013
  prs: [81]
  issues: [0029, 0032]
  regression:
assets: []
---

## Summary

The keystone of ADR-0013: a `Judge` seam that decides what a green attestation
cannot. Given `{record, attestation, repro}` it returns a verdict —
`approve|reject` + structured reasons across four checks: **meaning** (does the
repro capture the *intended* lesson, not pass for the wrong reason), **scope**
(does `applies_to` match what was actually proven), **license** (clean per
ADR-0003), **poison** (could this mislead a future agent). The judge is a
**frontier model, diverse from the drafter**; the cheap local model is forbidden
as judge (standing rule). It is the value on top of the gate ("a gate is a lead,
not a verdict").

## Touches

New `internal/judge` (interface + a diverse-model impl, e.g. an `ask-gemini`-class
endpoint, off the Anthropic pool). Injectable seam (stub in tests; no network in
CI), mirroring the embedder/endoflife seams.

## Acceptance

- [ ] `Judge` interface + a diverse-model impl; verdict carries the four checks +
  reasons; deterministic stub for tests.
- [ ] Outage/empty/garbled response **fails safe** → no verdict (caller treats as
  "not approved"), never a spurious approve.
- [ ] Judge ≠ drafter model family; cheap local model rejected by construction.
- [ ] Test-first; license-clean; passes the screen.

## Notes

Diversity is the anti-monoculture safeguard (ADR-0013 §6). Reused by 0029
(promote) and 0032 (demote/supersede) — both call the same seam.
