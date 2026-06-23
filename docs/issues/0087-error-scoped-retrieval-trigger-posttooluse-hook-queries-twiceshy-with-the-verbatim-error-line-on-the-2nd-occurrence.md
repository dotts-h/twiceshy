---
id: 0087
title: Error-scoped retrieval trigger — PostToolUse hook queries twiceshy with the verbatim error line on the 2nd occurrence
status: closed
severity: high
group: 0064
depends_on: []
forgejo: 363
links:
  adr:
  prs: [370]
  issues: []
  regression:
assets: []
---

## Summary

Move the retrieval trigger off agent judgment and off per-prompt push, onto the
moment it actually targets: **an error appearing in tool output**. A consuming-agent
`PostToolUse` hook greps Bash/tool results for error signatures (`Error`, `Traceback`,
`panic`, `TypeError`, `error:`, `[!]`, non-zero exit) and, on a hit, calls
`search_experience` with the **verbatim error line** (which is exactly what twiceshy
indexes as `error_signatures`), injecting any hits as `additionalContext`.

This fixes both observed failure modes at once (field report, RN session):
- **Pull never fires** — an in-context "search before debugging" instruction loses to
  "I'll just read the error." Judgment can't be nudged reliable; move the trigger off it.
- **Push is 0% precision** — per-prompt firing is the wrong signal (every turn). Errors
  are orders of magnitude rarer, so the precision tax that justified deferring push
  disappears, and the query is higher-signal than any an agent hand-builds.

## Repro
1. RN/iOS session hit `TypeError: Cannot read property 'lngLat' of null` and
   `[!] Unable to find a specification for 'RCT-Folly'` — both qualifying moments.
Expected: twiceshy consulted with the verbatim error line.
Actual: pull never fired (agent diagnosed from ground truth without querying);
per-prompt push injected 8 off-domain cards (0/8 precision).

## Evidence
Field report (2026-06-23 RN session); twiceshy's own `exp-0622` (df-gate leaks
common words) and `exp-0001` (FTS5 MATCH needs tokenizing raw input).

## Notes
Design caveats (make-or-break): dedupe/debounce per distinct signature; gate on the
**second** occurrence ("before retrying what just failed" is the cleanest tripwire);
filter expected non-zero exits (grep no-match, a test meant to fail, an anticipated
404); empty result = success, inject nothing.

**Server prerequisite (blocking):** `search_experience` must survive raw error
strings full of FTS5-hostile punctuation (`RCT-Folly`, `node.js`, `@scope/pkg`, `[!]`,
`modernc.org/sqlite`). The fix is `exp-0001` (tokenize + quote each token, never hand
raw text to MATCH) — verify/harden that path against error-shaped input and add a guard
test before shipping the hook.

**Measure it (depends on #0005):** justify on precision/recall of **error-moment**
firings — a different distribution from per-prompt — before trusting it. Belongs to
the #0064 agent-native feedback-loop epic; complements #0069 (helpfulness signal).

## Resolution (done 2026-06-23)

Shipped the trigger — the hook *and* its blocking server prerequisite — as a
fail-open consuming-agent hook over a hardened, guard-tested search path. The hook
prototype landed earlier; this finalizes and de-prototypes it.

- **Server prerequisite (the blocker), done:** the verbatim error line the hook
  sends is dense with FTS5-hostile punctuation (`RCT-Folly`, `node.js`,
  `@scope/pkg`, `[!]`, `modernc.org/sqlite`). exp-0001's tokenize-and-quote path
  already escapes every token, but `RetrievePush` (the `/push` channel the hook
  uses) was *not* under the never-errors property. Added
  `TestErrorScopedQueriesSurviveHostileInput` (`internal/index/error_input_test.go`)
  pinning the field-report lines through `RetrievePush` + `Search`, and extended
  `FuzzSearchNeverErrors` to cover `RetrievePush` and seed it with those lines
  (15 s / 38 k execs, zero crashes).
- **Hook, shipped (de-prototyped):** `hooks/twiceshy-error-pull.sh` fires on the
  *second* distinct error signature (the "before retrying what just failed"
  tripwire), dedupes per session+signature, rejects benign prose via the
  trailing-colon anchor, fails open throughout, and injects nothing on an empty
  result. Now covered by `hooks/twiceshy-error-pull.test.sh` (8 cases, shellcheck
  clean) with a `PostToolUse` registration example in `hooks/README.md`.
- **Design note:** the hook queries `/push`, not raw `search_experience`, on
  purpose — `/push` renders matching cards as `additionalContext` and applies the
  discriminative-token gate (#0068), which is exactly what an *automatic* injection
  hook (no agent in the loop) wants; an error line's rare identifiers are the
  tokens that gate is built to pass.

**Measurement rides on #0005** (this issue's "Measure it" note): the error-moment
precision/recall of these firings — a different distribution from per-prompt push —
is folded into the #0005 trap-avoidance eval, not asserted here.
