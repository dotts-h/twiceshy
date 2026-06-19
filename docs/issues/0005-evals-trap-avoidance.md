---
id: 0005
title: Trap-avoidance eval suite — memory on/off regression for the store
status: in-progress
severity: medium
group: 0008
depends_on: []
forgejo: 95
links:
  adr: docs/adr/ADR-0001-architecture.md
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
The project's regression suite for the store itself (Phase 5): walk an agent
toward each recorded trap with memory **on vs off**, and score avoidance plus
steps/tokens. Publishable novelty — no published suite measures this
(ADR-0001 §8).

## Scope
- [ ] Harness: drive an agent toward each `trap`/`dead-end` record, memory on/off. *(slice 2)*
- [ ] Metrics: avoidance rate, steps-to-solution, tokens; per-record and aggregate. *(slice 2)*
- [x] Wire into `make ci` (or a separate target) as the store's regression gate. *(`make eval` target; report-only, not blocking — recall shifts as the corpus grows)*
- [x] Report the near-miss failure mode explicitly (does a related-but-wrong card hurt?). *(near-miss rate + per-case wrong-card reporting)*

## Progress (slice 1, 2026-06-19)

`internal/eval` + `twiceshy eval` ship the **retrieval-effectiveness** slice: the
cheap, deterministic precondition for the agent eval. It drives the same
validated-only pull path an agent uses, with queries taken from each behavioral
record's error signatures (the text an agent sees) + symptom summary, and reports
**recall@k, MRR, near-miss rate** per-case and aggregate. No LLM budget.

First run on the live corpus (18 cases over the 6 validated behavioral records):
**recall@3 = 100%, MRR = 0.972, near-miss = 5.6%**. The one near-miss is genuine
ambiguity, not a defect: the bare signature `"permission denied"` surfaces
exp-0004 (NAS bind-mount perm-denied) above exp-0017 (noexec-TMPDIR perm-denied)
— both legitimately match. Evidence that retrieval works on the validated set.

**Slice 2 (remaining):** the GitChameleon-style agent-task eval — does the
retrieved card change task success / steps / tokens (memory on vs off).

## Notes
**Re-scoped off #0002 (2026-06-19):** the eval measures the PULL path
(`search_experience`), which IS the injection path — push (#0002) was deferred, so
the old dependency is stale. Removed `depends_on: [0002]`. Still uses a non-trivial
corpus (#0007 + the live feed).
