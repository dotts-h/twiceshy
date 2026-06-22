---
id: 0078
title: Interim: take corpus imports off the code CI (shim + relax outdated-branch + slower cadence)
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
Interim (ADR-0021 option D): a required-check SHIM for data-only changes + relax block_on_outdated_branch + slower import cadence, so corpus imports stop running the full Go CI and stop forcing code-PR rebases. Do-now; not throwaway (we want imports off code CI regardless).
