---
id: 0029
title: Auto-promotion — attestation + judge PASS promotes quarantined to validated (no human approver)
status: closed
severity: high
group: 0027
depends_on: [0028]
forgejo:
links:
  adr: ADR-0013
  prs: [84]
  issues: [0026, 0012]
  regression:
assets: []
---

## Summary

The positive direction of ADR-0013: for an execution-provable record, a **holding
broker attestation** (#0020) **+ a judge PASS** (#0028) flips `quarantined →
validated` with **no human approver** — recording the attestation id + verdict in
`provenance` and setting `validated_at`. Git + CI stay the boundary and audit
trail: the promotion is a commit that rides the ADR-0012 self-merge PR flow (bot
opens → CI green → self-merge), reversible by supersede.

## Touches

`internal/repro`/`internal/drafter` (wire judge into the post-attestation step) +
a promotion path (a `doctor promote`, or extend the drafter `Pipeline` / `twiceshy
draft`) + `cmd/twiceshy`. The flip is a per-record delta (ADR-0001 §7), never a
whole-store rewrite.

## Acceptance

- [ ] Holding attestation + judge PASS → record becomes `validated` with
  `validated_at` + attestation id + verdict in provenance; schema-valid.
- [ ] Judge REJECT or no verdict → stays `quarantined` (fail-safe); reason logged.
- [ ] Only **execution-provable** records are eligible; non-provable records are
  left for a human (ADR-0013 §5).
- [ ] Promotion is auditable (git commit + provenance) and reversible (supersede).
- [ ] Test-first (stubbed judge + attestation); `make ci` green.

## Notes

Refines ADR-0010's propose-only stance into "apply-when-proven-and-judged" for the
provable class. Turns the #0026 output (exp-0043/0045) from dead weight into served,
validated records once they pass the judge.
