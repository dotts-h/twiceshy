---
id: 0110
title: Promotion judge gains a usefulness dimension: would this card change a competent agent's action
status: closed
severity: medium
group: 0106
depends_on: []
forgejo: 476
links:
  adr: docs/adr/ADR-0020-prose-class-panel-promotion.md
  prs: [479]
  issues: [0106]
  regression:
assets: []
---

## Summary
The promotion panels judge **Meaning/Scope/License/Poison**
(`internal/judge/judge.go:54`, `Checks`) — nothing asks whether the record would
ever change an agent's action. Content-shaped non-lessons pass every existing
check and reach `validated`: exp-2845 ("a selftest was added once", `kind:
convention`) is internally coherent, in-scope, license-clean, and non-misleading —
it just isn't useful — and was panel-approved on 2026-06-28. Every such record
becomes push fodder (until #0107 also excludes it by kind/origin) and dilutes the
corpus the df gate measures rarity against (epic #0106). Add a fifth check,
**Usefulness**: "would this card plausibly change a competent coding agent's next
action in a matching session?"

## Repro
1. Replay exp-2845's content through the prose panel judge.
Expected: rejected — the content never changes what an agent does next.
Actual: approved on all four existing checks (2026-06-28); no check asks the
usefulness question, so a passing-but-useless record has no path to rejection.

## Evidence
- `internal/judge/judge.go:54` (`Checks = []CheckName{Meaning, Scope, License,
  Poison}`) is the canonical, ordered set every complete verdict must cover;
  `Verdict.Approved()` (`internal/judge/judge.go:78`) already requires every
  canonical check to be present and passing, so **extending `Checks` extends the
  gate** — no new plumbing needed in `Approved()` itself.
- `internal/judge/model.go:366` (`BuildAdvisoryPrompt`) and `:427`
  (`BuildProsePanelPrompt`) render the four-check rubric to the judge model
  today; neither prompt asks about downstream action.
- `internal/judge/system.go` (`AdvisorySystemV1`, `ProsePanelSystemV1`) are the
  system-prompt instructions gating the advisory and prose panel paths
  respectively (ADR-0016, ADR-0020) — both need the fifth check added.
- exp-2845 ("Use Selftests for Argument Parsing Invariants"), `kind: convention`,
  panel-approved 2026-06-28 — cited in epic #0106 as the concrete non-lesson that
  slipped through.

## Acceptance
- A fifth `CheckName = "usefulness"` is added to `internal/judge/judge.go:54`'s
  `Checks`, gating both the advisory panel (`BuildAdvisoryPrompt`, ADR-0016) and
  the prose panel (`BuildProsePanelPrompt`, ADR-0020) paths — `Approved()`
  requires it like the other four, fail-safe by construction (a missing or
  failing usefulness check blocks promotion, same as any other canonical check).
- exp-2845's content, replayed against the updated prompt/rubric, is rejected on
  usefulness.
- The judgeeval gold set (`internal/judgeeval/gold.yaml`) gains cases: an
  exp-2845-class reject (useless-but-otherwise-clean) and a genuine trap approve
  (useful and clean), so the fifth check has both a positive and a negative
  anchor.
- Existing gold-set cases still pass (the new check must not regress the four
  existing ones); judge prompt-pinning tests are updated for the changed system
  prompts.

## Notes
This is complementary to #0107, not redundant with it: #0107 filters the
*existing* corpus by kind/origin (what may reach push today); this issue narrows
what enters `validated` at all going forward, at the promotion boundary that
produced exp-2845 in the first place.

**Amended at close-out (PR #479):** the shipped design applies the usefulness
check to **all three** judge paths — proof (`ProseSystemV1`→V2), advisory
(`AdvisorySystemV1`→V2), and prose-panel (`ProsePanelSystemV1`→V2) — not just
advisory/prose-panel as originally scoped above. `Verdict.Approved()`
(`internal/judge/judge.go:78`) fail-safes on every canonical check regardless of
path: a path whose prompt doesn't ask the usefulness question would still have
`Usefulness` demanded by `Approved()` and so would have every one of its
promotions blocked outright, not exempted. Per-path check vocabularies are
fragile for this reason — extending `Checks` extends the gate for every path
by construction, so the fix had to extend every path's prompt, not just two of
them. The execution-provable/proof path's original rationale ("a passing repro
is itself evidence of an actionable claim") turned out not to be a valid
carve-out under a canonical-check gate shared across paths.
