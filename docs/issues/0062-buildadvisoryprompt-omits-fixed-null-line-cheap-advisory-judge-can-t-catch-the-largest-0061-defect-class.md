---
id: 0062
title: BuildAdvisoryPrompt omits fixed:null line — cheap advisory judge can't catch the largest #0061 defect class
status: closed
severity: medium
group: 0015
depends_on: []
forgejo: 194
links:
  adr: ADR-0016
  prs: [263]
  issues: [0061]
  regression:
assets: []
---

## Summary

`internal/judge/model.go` `BuildAdvisoryPrompt` renders the `fixed:` line only when
`at.Versions.Fixed != nil`, so a `fixed: null` record shows **no** `fixed:` line at
all in the prompt. The off-pool advisory judge (gpt-oss/gemini) therefore can't
reliably catch the `fixed:null` + boilerplate "upgrade past the fixed version"
contradiction — which is the **single largest #0061 defect class (10 of 19)**.

A full-file Sonnet audit caught these because the agent read the raw YAML (explicit
`fixed: null`); the production judge, seeing only the rendered prompt, is at a
structural disadvantage and must infer the contradiction from the *absence* of a
fixed line plus the fix text.

## Repro
1. Take a `fixed:null` advisory record (e.g. exp-0012) and render it through
   `BuildAdvisoryPrompt`.

Expected: the prompt makes the missing fix explicit, e.g. `fixed: (none published)`,
so the judge can see "fix text says upgrade past a fixed version, but none exists".

Actual: no `fixed:` line is emitted; the contradiction is only inferable from
absence, which the cheap judge frequently misses.

## Evidence

`internal/judge/model.go` ~L313: `if at.Versions.Fixed != nil { ... "  fixed: %s" }`
— the nil branch emits nothing. The 2026-06-20 Sonnet audit
(`runs/sonnet-advisory-audit.json`) found 10/19 rejects in this class (#0061
Defect 3); the gpt-oss+Composer head-to-head (#0061 evidence) showed the cheap
judges approving several of them.

## Notes

Fix: emit an explicit marker when `Fixed == nil` (e.g. `fixed: (none published)`),
and add a gold-eval case (blocked on #0063 advisory-prompt routing) asserting the
judge rejects the `fixed:null`-contradiction shape. Small, surgical change to the
advisory prompt; re-pin any judge-eval snapshot after. Related: [[ADR-0016]], #0061.
