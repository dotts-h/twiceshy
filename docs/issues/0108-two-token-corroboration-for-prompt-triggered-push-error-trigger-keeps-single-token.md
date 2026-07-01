---
id: 0108
title: Two-token corroboration for prompt-triggered push; error trigger keeps single-token
status: closed
severity: high
group: 0106
depends_on: []
forgejo:
links:
  adr: docs/adr/ADR-0028-push-eligibility-and-corroborating-specificity.md
  prs: [480]
  issues: [0106, 0107, 0069]
  regression:
assets: []
---

## Summary
`ftsQuery` joins tokens with **OR** (`internal/index/index.go:717-729`), so
`discriminativeTokens` finding ≥1 discriminative token opens the gate, but the
*served* card only has to match ONE of them — a single accidentally-rare word is
enough. Live specimen: prompt *"need a deep analysis of this application and why
it is still not working well not helping any llm"* fired on `application` (df=2) +
`llm` (df=2) and served two unrelated cards (a Docker-Compose trap + a selftest
convention card), **neither of which contains "llm"**. Prompt-triggered push must
require **corroboration**: the gate opens on ≥2 discriminative tokens, and each
*served* card must itself lexically match ≥2 distinct discriminative tokens — not
just be reachable via the OR-joined query. Error-triggered push (the PostToolUse
error-pull hook, `hooks/twiceshy-error-pull.sh`) keeps single-token semantics: it
queries on a verbatim error line, which is high-signal by construction (#0087).
Fingerprint-exact bypass (`RetrievePushTraced` step 1) is unchanged.

## Repro
1. `curl` `POST /push` with `{"query": "need a deep analysis of this application
   and why it is still not working well not helping any llm"}` against the live
   validated corpus.
Expected: 0 cards served — the prompt is off-topic meta-commentary, not a real
error or trap-triggering context.
Actual: 2 cards served (a Docker-Compose trap + exp-2845's selftest-convention
card), matched purely on `application`/`llm` each independently clearing df≤3;
neither served record's text contains the word "llm".

## Evidence
- `internal/index/index.go:717-729` (`ftsQuery`): tokens joined with `OR` — "for
  recall on long error texts", per its own comment; that recall property is
  exactly what turns one accidental token into a full card serve.
- `internal/index/index.go:398` (`RetrievePushTraced`) step 2:
  `discriminativeTokens` gates on `len(disc) == 0`, i.e. **any** discriminative
  token count ≥1 opens the gate today; step 3 then searches the OR-joined
  discriminative set with no per-hit corroboration.
- The live specimen above (epic #106's diagnosis session, 2026-07-01).
- `hooks/twiceshy-error-pull.sh` and `hooks/twiceshy-push.sh` both currently
  `POST /push` with `{query: $q}` — no field distinguishes an error-triggered call
  from a prompt-triggered one server-side.

## Mechanics
- `POST /push` (`internal/server/push.go`, `PushArgs`) gains an optional `trigger`
  field, `∈ {"", "prompt", "error"}`; **empty defaults to `"prompt"` (strict)
  semantics**, so old clients fail closed rather than silently keeping the loose
  single-token behavior.
- `hooks/twiceshy-error-pull.sh` sends `trigger:"error"` **and** `session` — it
  currently sends neither (payload is `{query: $q}` only); the missing `session` is
  also an #0069 attribution gap (push's own hook already sends `session`, per
  `PushArgs.Session`, `internal/server/push.go:30`).
- `hooks/twiceshy-push.sh` sends `trigger:"prompt"` explicitly.
- `docs/CONTRACTS.md`'s `POST /push` row is updated for the contract change (new
  field, its default, and the per-trigger corroboration semantics it selects).

## Acceptance
- The live specimen prompt above serves **0** cards under `trigger:"prompt"`
  (or the default).
- On-topic multi-token error queries still serve under `trigger:"error"`.
- A genuine single-discriminative-token error-pull query still serves (single-token
  semantics preserved for `trigger:"error"`).
- The precision/recall guard (`TestPushPrecisionOnLiveCorpus`,
  `internal/eval/eval_livecorpus_test.go`, cases from `eval.PushNegatives()` /
  `eval.PushPositives()`, `internal/eval/eval.go:164`/`191`) is extended with the
  specimen (and its trigger variants) and stays green.

## Notes
Corroboration is computed over the eligible subset from #0107 — a served card must
match ≥2 distinct discriminative tokens **and** be push-eligible; the two issues
are additive, not sequenced (no `depends_on`, per the epic's "parallelizable
children" framing, though implementing both together against the same
`RetrievePushTraced` seam is the natural order). Fingerprint bypass is unaffected
either way — a deterministic stack signature needs no corroboration.
