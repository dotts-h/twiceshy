---
id: 0128
title: Write-path hardening for untrusted contributors: hostile-input PII/secret scrub + low-trust origin tier
status: closed
severity: medium
group: 0124
depends_on: []
forgejo:
links:
  adr:
  prs: [518]
  issues: []
  regression:
assets: []
---

## Summary

Harden the write path for hostile input before `record_experience`/
`report_outcome` open to alpha tokens: PII/secret scrubbing tuned for untrusted
authors (today's redact_pii assumes a cooperative caller), size/shape limits,
per-token contribution quotas, and a low-trust origin tier so anonymous-token
records land quarantined, flow the unchanged judge/soak pipeline, and can never
reach the push channel (ADR-0001 §4 floor; #0118 opened the per-origin
admission door).

## Notes

Gates the write-alpha, not the launch. Poisoning drills belong here: seed a
malicious draft and assert it cannot reach validated without tripping a check.

## Close-out (2026-07-06, PR #518)

Shipped: origin stamping, push exclusion for alpha origins, fail-closed
secret rejection, size caps, per-token contribution quotas, and a poisoning
drill. **The write tools are NOT yet enabled for alpha tenants** — this issue
was the prerequisite hardening; enabling `record_experience`/`report_outcome`
for alpha tokens is a deliberate later switch per ADR-0030 phase 2, not a
side effect of this close-out.
