---
id: 0020
title: internal/repro revalidate doctor — version matrix + attestation, report-only (Go first)
status: open
severity: high
group: 0015
depends_on: [0016, 0018, 0019]
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The validation harness proper (ADR-0011 §3): `internal/repro.Revalidator`
implementing `doctor.Doctor` — runs each record's **test-set** in the broker
across a **version matrix**, proves fail→pass, **empirically derives `applies_to`
bounds**, and emits a `Finding` + a structured **attestation** (what ran, image
digests, versions, results). **Report-only** — a human flips `validated` /
`validated_at` in the PR (promotion never automatic). Go ecosystem first.

## Touches

`internal/repro` (Revalidator); CLI `twiceshy doctor revalidate`.

## Acceptance

- [ ] Revalidator implements `Doctor`; report-only (no record mutation)
- [ ] Runs positive + negative tests across the matrix; attestation emitted
- [ ] Derives + sanity-checks `applies_to` version boundaries from results
- [ ] Unit-tested with a stub broker; integration test on one real Go record
- [ ] Depends on 0016 (schema), 0018 (broker), 0019 (screen)
