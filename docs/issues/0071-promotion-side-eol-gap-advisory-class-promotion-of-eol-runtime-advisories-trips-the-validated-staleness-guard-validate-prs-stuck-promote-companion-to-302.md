---
id: 0071
title: Promotion-side EOL gap — advisory-class promotion of EOL-runtime advisories trips the validated staleness guard (validate PRs stuck); promote companion to #302
status: open
severity: high
group: 0015
depends_on: []
forgejo: 304
links:
  adr: docs/adr/ADR-0016-advisory-class-panel-promotion.md
  prs: []
  issues: [0015]
  regression:
assets: []
---

## Summary
#302 scoped the D2 staleness guard to **validated** records (so quarantined imports
of EOL-runtime advisories no longer freeze the importer). The mirror gap is on the
**promote** side: the advisory-class panel (ADR-0016) promotes OSV/GHSA advisories to
`validated` **without checking EOL** — so when it promotes an advisory targeting an
end-of-life runtime (e.g. a Python-3.8 vuln), that record becomes a validated EOL
record, which the (now validated-scoped) staleness guard correctly flags →
`TestStaleness_RealCorpusNotFalseFlagged` goes red → the validate PR is stuck. As of
2026-06-22 there are **~36 stuck validate PRs**, almost certainly this.

## Repro
1. Let the advisory panel promote an EOL-runtime advisory (e.g. Python 3.8) to validated.
2. Run `make test`.
Expected: green.
Actual: `staleness_test.go` `TestStaleness_RealCorpusNotFalseFlagged` fails — a validated
record is EOL-flagged → the validate PR can't merge.

## Evidence
36 open `validate/*` PRs, mergeable but not landing (2026-06-22). Import side fixed by
#302; promote side unaddressed. Records like exp-0798 (aiohttp / Python 3.8 EOL 2024-10-01).

## Fix options (decide; ADR-0016 amendment)
- **A (recommended):** the advisory panel / `promote.Eligible` **excludes** an advisory
  whose runtime is already EOL — a born-stale advisory is not promote-worthy (it would be
  demoted on the next staleness run anyway). Keeps the validated corpus clean.
- **B:** promote it but stamp `valid.until`/`stale` immediately — pointless churn; rejected.
- **C:** loosen the guard test to tolerate validated EOL records — wrong; they *are* stale.

## Acceptance
- [x] A validate run that would promote an EOL-runtime advisory skips it (logged), so no
      validated record trips the staleness guard. — `promote.WithStalenessGate` holds the
      record (`held … runtime is end-of-life — born-stale (#0071)`) before the panel.
- [ ] The stuck validate-PR backlog clears (re-run after the fix; close the stale ones).
      — **post-merge / operational**: the next scheduled validate run uses the fixed binary
      and produces green PRs; the ~36 already-open stale PRs are then closed/superseded.
- [x] `make test` stays green as the corpus grows across ecosystems. — guard test
      `TestStaleness_RealCorpusNotFalseFlagged` + new `TestStaleness_WouldFlag_StatusIndependent`
      and `TestPromote_AdvisoryEOLRuntime_HeldNotPromoted` all green under `-race`.

## Resolution
Option **A** (charter): the advisory panel path is gated by the D2 staleness check.
`doctor.Staleness.WouldFlag` exposes the staleness signals status-independently (the
promote-side mirror of the validated-scoped guard #302 introduced); `promote.promoteAdvisory`
consults it and refuses a born-stale advisory **before** the costly panel. Wired in the
`validate` command against the real endoflife.date source (`TWICESHY_EOL_URL` overrides;
fails open with the guard test as backstop). Decision recorded as **ADR-0016 §7**
(amendment 2026-06-22). Dogfood lesson: `experience/2026/0807-promote-eol-gate.md`.
Code fix lands in this PR; only the backlog-clear box above is a post-merge step.

## Notes
Direct companion to #302 (the import-side fix). Relates to ADR-0011 (the validation
engine), ADR-0013 (promote/adapt), ADR-0016 (advisory panel). Found 2026-06-22 while
unfreezing the live importer.
