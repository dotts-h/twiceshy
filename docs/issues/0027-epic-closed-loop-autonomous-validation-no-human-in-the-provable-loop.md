---
id: 0027
title: "Epic: Closed-loop autonomous validation — no human in the provable loop"
status: closed
severity: high
group:
depends_on: []
forgejo: 117
links:
  adr: ADR-0013
  prs: []
  issues: [0015, 0020, 0026, 0010]
  regression:
assets: []
---

## Summary

Make twiceshy live its own premise (ADR-0013): a **proven** record goes live on
its own, and a lesson that **misfires in practice** feeds back and corrects the
corpus — **no human in the provable loop**. Today the #0015 engine *proves* repros
but promotion is human-gated and quarantined records aren't served, so the engine
produces dead weight whenever no human is standing by; and there is no reverse
channel when a served card steers an agent wrong.

The shift is precise: keep git + CI as the boundary/audit trail, but replace the
**human approver** with **execution proof + a diverse-model judge** for
execution-provable records. The cheap local model is never the judge (standing
rule); non-provable records keep a human.

## Phasing (children)

Mostly **disjoint seams → parallel lanes**. `internal/judge` ⟂ `internal/index`+
`server` (usage) ⟂ `server`+`ingest` (intake); the two promote/demote wirings reuse
`internal/repro`+`drafter`.

1. **0028 — Judge seam** (the keystone): a `Judge` interface + a diverse frontier-model
   impl that, given {record, attestation, repro}, returns a verdict
   (approve/reject + reasons: meaning · scope · license · poison). Injectable,
   stubbed in tests. Blocks 0029 and 0032.
2. **0029 — Auto-promotion (positive proof → validated)**: holding attestation +
   judge PASS → promote `quarantined → validated` via the ADR-0012 self-merge PR
   flow, recording the attestation id + verdict in provenance. `depends_on: 0028`.
3. **0030 — Usage signal**: retrieval increments `provenance.usage`
   (retrieved/last_hit; confirmed_helpful from a positive report) — unblocks
   ADR-0010's D4 and gives the loop a reinforce/decay signal. Independent lane.
4. **0031 — Outcome-report intake**: MCP `report_outcome` tool — a consuming agent
   submits {record_id used, outcome, failing repro/error}; stored as a quarantined
   counter-record / revalidation request, screened like any ingest. Propose-only.
   Independent lane.
5. **0032 — Counter-evidence gate + adapt (negative proof → stale/supersede)**:
   turn a report into a repro, re-run original + counter through the broker; the
   judge approves demote/supersede when the claim breaks; non-reproducing reports
   *accumulate* into a `disputed` escalation (don't drop the prod-only failures the
   sandbox can't reproduce); `applies_to` narrowing is judge-gated + reversible.
   `depends_on: 0028, 0031`.
6. **0033 — Guardrails** (ADR-0013 §7, from the diverse-model review): anomaly
   monitoring + an emergency stop + budget caps — the layered cover for an
   available-but-compromised judge and a `report_outcome` DoS. `depends_on: 0029, 0031`.

## Definition of done

A drafted+proven record reaches `validated` and is served **with no human PR**; a
`report_outcome` that reproduces against a card's claim demotes/supersedes it **with
no human PR**; both paths are git-committed, CI-gated, judge-recorded, and
reversible by supersede. Non-provable records still route to a human.

## Build discipline

Slices are built **executor-first** (the hardened operating mode): Claude writes
the spec + the failing test gate + judges the diff and integrates; the
non-Anthropic executor (composer/`code-exec`) does the wiring; the diverse judge
itself is `ask-gemini`-class, off the Anthropic pool. `make ci` + the human PR (for
the *code*, not the records) stay the safety net.

## Notes

Grounding: ADR-0013 (the decision), ADR-0011 (the engine this closes the loop on),
ADR-0010 (the propose-only stance this refines), ADR-0012 (the self-merge mechanism
promotion rides). Sibling epic to #0015 — that made "validated" mean "we ran it";
this makes promotion and correction autonomous.
