---
id: 0107
title: Push eligibility: only agent-actionable origins and kinds reach the push channel
status: closed
severity: high
group: 0106
depends_on: []
forgejo:
links:
  adr: docs/adr/ADR-0028-push-eligibility-and-corroborating-specificity.md
  prs: [480]
  issues: [0106, 0108]
  regression:
assets: []
---

## Summary
The push channel's corpus is demand-irrelevant: **~940/990 validated records are
importer-origin advisories** (self-audit material, never mid-prompt material) and
the prose-panel promotion path validates generic non-lessons (exp-2845 "a selftest
was added once", `kind: convention`, panel-approved 2026-06-28). Every one of these
is currently push fodder. Push-eligibility should mean **agent-actionable at the
moment a prompt is being written**: `kind ∈ {trap, fix}` AND
`provenance.source.author` (lowercased) is not an importer origin
(`{twiceshy-importer}`). Pull (`search_experience`) and `Assess` are **unchanged** —
this scopes the push channel only, per ADR-0001 §3–4's push/pull split.

## Repro
1. Query the live validated corpus for `kind`/`provenance.source.author` distribution.
Expected: push serves only records an agent mid-prompt would act on.
Actual: ~95% of validated records are OSV-importer advisories
(`provenance.source.author: twiceshy-importer`, `cmd/twiceshy/main.go:569`'s default);
none of them are trap/fix material an agent needs while writing code, yet all are
push-eligible today.

## Evidence
- Epic #0106's corpus-composition finding (~940/990 importer-origin).
- `internal/record/record.go:678` requires `provenance.source.author`; the importer
  stamps it via the `-author` flag defaulted to `twiceshy-importer`
  (`cmd/twiceshy/main.go:569`).
- The `records` table DDL (`internal/index/index.go:223`) has no origin column
  today — `insertRecord` (`internal/index/index.go:328`) never persists it, so the
  eligible-subset filter cannot be computed from the index as it stands.

## Acceptance
- The `records` table indexes **origin** (lowercased `provenance.source.author`) at
  `Rebuild` (`internal/index/index.go:306`/`insertRecord`); a pre-existing DB
  without the column rebuilds cleanly (additive DDL, like the existing
  `usage.pushed` in-place migration noted in `docs/CONTRACTS.md:26`).
- Push retrieval (`RetrievePush`/`RetrievePushTraced`, `internal/index/index.go:398`
  onward — both the df gate's `validatedDF` count and the served-subset search) is
  computed over the **eligible** subset only: `status = 'validated' AND kind IN
  ('trap','fix') AND origin NOT IN (<importer origins>)`.
- The fingerprint-exact bypass (`RetrievePushTraced` step 1) stays unrestricted — a
  deterministic stack signature is real context by construction (ADR-0015) and is
  not scoped by kind/origin.
- Live-corpus guard: a query that today serves an importer-origin advisory card via
  push serves 0 cards via push after this change; the same query via
  `search_experience` (pull) still returns it.

## Notes
Scoping this to push only (not pull/Assess) preserves ADR-0004/0007's pull-floor
invariants untouched — see #0106 (epic) and ADR-0028 (this decision's record). The
`kind ∈ {trap, fix}` cut and the usefulness question #0110 asks at promotion time
are complementary, not redundant: this issue filters what already exists in the
corpus; #0110 narrows what enters it going forward.
