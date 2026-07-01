---
id: 0080
title: Stand up the twiceshy-corpus store + its CI + the no-silent-freeze stall alarm
status: closed
severity: high
group: 0076
depends_on: [0077]
forgejo: 453
links:
  adr: ADR-0021
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
ADR-0021 phase 2: create the corpus store from the snapshot (C0077), with its OWN CI (schema-validate + validated-scoped doctors, exp-0746) and a stall alarm that never swallows an auto-merge result. Not yet authoritative.

## Outcome (2026-06-22) — store stood up (NOT yet authoritative)

New repo **`claude/twiceshy-corpus`** (private) created and populated:
- **Lossless import**: 2694 records imported from the snapshot tag
  `corpus-snapshot-pre-decouple-20260622`; the repo's `experience/` tree is
  **byte-identical** (`cac6dfc`) to the engine's snapshot tree. Carries the schema
  contract (`schema/`, `docs/SCHEMA.md`) + a README.
- **CI** (`.forgejo/workflows/validate.yml`, **green** on its first run, PR #1):
  schema-validate + structural integrity as a HARD gate (`twiceshy index` = strict
  `LoadCorpus`: schema_version, dup ids, superseded_by, repro presence), and the
  validated-scoped staleness doctor **report-only** (exp-0746 — a quarantined draft
  can never red the gate). The validator is **built from a pinned engine commit**
  (`ENGINE_SHA=f4aff9e`, guard 3 — corpus CI consumes the engine, doesn't re-implement
  it); runs on the generic `ubuntu-latest` runner (a data repo needs no gVisor sandbox).
- **Branch protection** applied (required check = the corpus CI; 0 approvals for
  self-merge; `block_on_outdated_branch:false`).

### Deferred to #0081 (cut-over), by dependency — not skipped
- **Stall alarm**: the existing `scripts/corpus-stall-alarm.sh` is env-configurable
  (`TWICESHY_FORGEJO_API` → any repo) and already tested; but it watches `import/*` /
  `validate/*` PRs, which only flow to the corpus repo once the importer is re-pointed
  at cut-over. The brain timer instance for `twiceshy-corpus` is therefore instantiated
  in #0081 (nothing to watch until then). Mechanism is proven; only the live wiring waits.
- **Hardening**: the corpus CI's engine-read credential currently reuses the `claude`
  token (already present in the brain's git remotes; encrypted Actions secret, trusted
  runner). Scope it to a read-only token before multi-tenant (#0010).

Nothing reads this store yet — the engine repo remains authoritative until the #0081
cut-over. Reversible: delete the repo.
