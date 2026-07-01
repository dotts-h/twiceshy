# ADR-0028: Push eligibility + per-record corroboration restore push precision

- **Status:** Accepted (deciders: claude drafted from the 2026-07-01 live-telemetry
  diagnosis; horia ratified).
- **Related:** ADR-0015 (the discriminative-term gate this refines — the
  df-structural insight stands; its calibration clause, "re-confirm when validated
  crosses ~30", was never honored through 30→990), ADR-0017/#0068 (the global-IDF
  endgame — unchanged, still proposal-only), ADR-0004/ADR-0007 (pull's relevance
  floor — untouched, this decision scopes push only), ADR-0026 (runtime enforcement
  of experience adoption — why push must stay on rather than be disabled), #0106
  (the epic this ADR is filed under), #0107–#0111 (the children this decision
  authorizes), #0067 (the gate-decision telemetry that is the evidence base).

## Context

Live gate-decision telemetry (2026-06-24 → 07-01, v0.2.8) measured the push
channel's precision has collapsed: **70% of push queries inject cards** (504/723
across 85 sessions), against lifetime usage counters of **12,304 pushed / 1
confirmed_helpful**. The channel built to make twiceshy fire without agent
discretion is instead training every agent to ignore the `EXPERIENCE DATA` block —
the exact failure mode ADR-0015 was written to prevent.

Two compounding, independently-verified causes:

1. **The df gate inverted as the corpus grew.** ADR-0015's discriminative-token
   gate (`df ∈ [1, pushMaxDF=3]` over validated records) was calibrated on a
   ~30-record curated corpus and never re-confirmed — ADR-0015's own consequence
   clause names this exact re-validation as required "when the validated corpus
   crosses ~30" — as validated grew to 990 (~95% OSV-importer advisories). At that
   scale, ordinary dev words become "rare": a sampled 47-word set of everyday dev
   vocabulary measured **17 discriminative** (`rename`, `button`, `email`,
   `deploy`, `menu`, `login`, …), while genuinely topical tokens (`go`, `sqlite`,
   `react`) exceed df=3 and are excluded. The gate now *prefers accidental rarity
   over topical relevance*.
2. **The corpus feeding push is demand-irrelevant.** ~940/990 validated records
   are importer-origin advisories (self-audit material, never mid-prompt
   material), and the prose-panel promotion path validates generic non-lessons
   (exp-2845, "a selftest was added once", `kind: convention`, panel-approved
   2026-06-28). Every such record is push fodder today.

A third, structural failure mode compounds both: `ftsQuery` joins tokens with
**OR** (`internal/index/index.go:717-729`, "for recall on long error texts"), so
a discriminative-token *count* ≥1 opens the gate but a served card only has to
match ONE of the OR-joined tokens. Live specimen: prompt *"need a deep analysis
of this application and why it is still not working well not helping any
llm"* fired on `application` (df=2) and `llm` (df=2) and served two unrelated
cards — **neither served record contains "llm"**. One accidentally-rare token is
enough to serve a card today; the token count the gate checks and the tokens a
served card actually matches are two different things.

## Options considered

1. **Keep patching the stoplist.** Already failed twice — exp-0622 (the original
   trap) and tonight's collapse are the same root cause recurring at a larger
   scale. Hand-tuned, never provably complete, and requires a human to notice the
   next leak before patching it. Rejected as the primary fix (kept as a bridge,
   see #0111).
2. **Implement ADR-0017's global IDF now.** The right endgame — corpus-size-
   independent specificity from a real dev/code background corpus — but requires
   sourcing/licensing an external dataset (StackOverflow/GitHub-scale, likely
   CC-BY-SA) and an offline precompute pipeline. Too slow to stop a live bleed
   tonight. Stays the endgame (#0068 remains proposal-only; this ADR does not
   supersede it).
3. **Eligibility filter + per-record corroboration (CHOSEN).** Structural,
   embedding-free, uses signals already present in the corpus: provenance origin
   (already required by schema, `internal/record/record.go:678`), `kind`, and
   token co-occurrence within a served record's own indexed text. Reversible —
   both filters can be loosened or removed without touching the corpus itself.
4. **Disable push.** Rejected: push is measured as the *only* channel with
   adoption (ADR-0026's 33h measure: push ~2,462×, pull 2–5×, feedback 0× —
   "anything left to model discretion does not happen"). Turning it off returns
   to the pre-ADR-0015 problem this whole line of ADRs exists to solve.

## Decision

Adopt Option 3. Numbered:

1. **Push-eligible records:** `kind ∈ {trap, fix}` AND
   `provenance.source.author` (lowercased) is not an importer origin
   (`{twiceshy-importer}`). Origin is indexed on the `records` table
   (`internal/index/index.go:223` DDL, populated in `insertRecord` at
   `Rebuild`, `internal/index/index.go:306`). Pull (`search_experience`) and
   `Assess` are **unaffected** — ADR-0004/ADR-0007's pull-floor invariants stand.
2. **Gate df is computed over the push-eligible validated subset.** Specificity
   is measured against what can actually be served, not against the full
   990-record corpus (~95% of which is never push-eligible under decision 1).
3. **Prompt-triggered push requires per-record corroboration.** The gate opens
   only with ≥2 discriminative tokens, AND a served card must itself lexically
   match ≥2 distinct discriminative tokens — closing the OR-join gap where one
   accidentally-rare token serves an unrelated card. Error-triggered push
   (`trigger:"error"`, the PostToolUse error-pull hook, #0087) keeps ≥1-token
   semantics: a verbatim error line is high-signal by construction, and
   requiring two tokens would silence its rarest, most valuable single-identifier
   hits. Fingerprint-exact bypass (`RetrievePushTraced` step 1) is unchanged for
   both triggers — a deterministic stack signature is real context by
   construction (ADR-0015).
4. **`POST /push` gains `trigger ∈ {"", "prompt", "error"}`** (`PushArgs`,
   `internal/server/push.go:21`); empty defaults to `"prompt"` (strict)
   semantics, so old clients fail closed rather than silently keeping today's
   loose behavior. Hook clients send their trigger explicitly:
   `hooks/twiceshy-push.sh` sends `trigger:"prompt"`;
   `hooks/twiceshy-error-pull.sh` sends `trigger:"error"` **and** `session` (it
   currently sends neither — the missing session closes an #0069 attribution
   gap as a side effect). `docs/CONTRACTS.md`'s `POST /push` row is updated for
   the new field.
5. **Gate-decision telemetry gains flag-gated raw query text**
   (`-telemetry-query-text`, truncated 256 bytes, default OFF) — the query hash
   alone was insufficient to diagnose tonight's collapse without pulling a
   session transcript out-of-band; single-tenant deployments can opt in.

## Consequences

- **Precision is restored structurally**, not by word-list patching: eligibility
  and corroboration both derive from signals already in the corpus (origin, kind,
  token co-occurrence), not from an ever-growing manual stoplist.
- **The known false-negative class widens.** A genuine single-rare-token
  *prompt* (not an error) no longer injects a card. Accepted under the existing
  "empty is an answer" invariant (ADR-0015) — error-pull and `search_experience`
  (pull) still cover that case.
- **The eligible subset is small** (~100 records, kind∈{trap,fix} minus importer
  origin) — ADR-0015's small-corpus caveat ("the df boundary is one document wide
  at small N") returns in miniature. Per-record corroboration is the guard against
  this at the current scale; ADR-0017's global IDF is still the cure. **Re-confirm
  the corroboration thresholds when the eligible subset crosses ~300** — written
  into the live precision/recall guard as an explicit corpus-size assertion
  (`TestPushPrecisionOnLiveCorpus`, `internal/eval/eval_livecorpus_test.go`),
  mirroring the exact lesson of ADR-0015's calibration clause that nobody re-ran:
  a calibration clause nobody re-runs is not a guard.
- **The live precision/recall guard is re-baselined** including the specimen
  prompts from this diagnosis (epic #0106), across both `trigger` values.
- **The usefulness check (#0110)** governs what enters the push-eligible pool
  going forward, complementing this ADR's filter on what already exists in the
  corpus.
- **Cost:** origin indexing is a one-time additive DDL migration at `Rebuild`
  (like the existing `usage.pushed` in-place migration); corroboration adds at
  most `MaxK × |discriminative|` (≤ 3×24) single-token MATCH lookups per served
  query — bounded, sub-ms, embedding-free (ADR-0001 §4 intact).
