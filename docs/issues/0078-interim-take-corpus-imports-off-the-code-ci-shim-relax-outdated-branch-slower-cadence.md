---
id: 0078
title: Interim: take corpus imports off the code CI (shim + relax outdated-branch + slower cadence)
status: closed
severity: medium
group: 0076
depends_on: []
forgejo: 451
links:
  adr: ADR-0021
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
Interim (ADR-0021 option D): a required-check SHIM for data-only changes + relax block_on_outdated_branch + slower import cadence, so corpus imports stop running the full Go CI and stop forcing code-PR rebases. Do-now; not throwaway (we want imports off code CI regardless).

## Outcome (2026-06-22) — DONE (phase-1 action line: shim + relax)

Implements ADR-0021's phase-1 action line ("the required-check **shim** + relax
`block_on_outdated_branch`"). Two of the three sub-items shipped here; the third
(cadence) is deferred — see below.

1. **Data-only CI shim** — `scripts/ci-data-only.sh` (+ `ci-data-only.test.sh`, 11 cases).
   `ci.yml` (`gates`) and `security.yml` (`sast`, `secret-scan`) gain a `Detect data-only
   change` step; the heavy Go/scan **steps** are gated on `steps.detect.outputs.skip != 'true'`.
   The **jobs always run and post their required status contexts** (a skipped required check
   posts no status and hangs the PR — the reason the workflows have no `paths-ignore`), so a
   data-only (`experience/`-only) PR goes green in seconds without `make ci` / `govulncheck` /
   `gitleaks`. **Fail-closed:** a push event, an empty/failed diff, or any non-`experience/`
   path → `skip=false` → the full pipeline runs (= status quo). Worst case is a missed skip —
   never a skipped gate on code, never a hung context.
2. **Relax `block_on_outdated_branch`** — `branch-protection.json` → `false` (config-as-code;
   applied to the live forge via `scripts/apply-branch-protection.sh`). A ~daily corpus import
   no longer forces every open code PR to rebase; imports don't interact with code.
   `require_linear_history` + `block_force_push` still hold.

### Deferred: slower import cadence
The OSV importer cadence is **not repo-tracked** — it's a brain-only `twiceshy-import.timer`
(no unit in `scripts/`). Tuning it is a live-brain ops change, and the importer is **stopped
outright at phase 3 (quiesce)** of the decouple, so slowing it now is churn against a moving
target (the observed frequent import-PRs vs. the timer's ~daily schedule isn't yet fully
mapped — don't tune blind). Folded into the phase-3 quiesce ops rather than done here.
