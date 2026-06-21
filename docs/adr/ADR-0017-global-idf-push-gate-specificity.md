# ADR-0017: A global dev/code IDF replaces the hand-maintained stoplist as the push gate's specificity signal

- **Status:** Proposed (deciders: claude, get-next session). An ADR *proposal*, not a
  silent change: it records the direction; the implementation lands in a follow-up
  gated by this decision. Score calibration is explicitly out of scope (below).
- **Related:** ADR-0015 (the discriminative-term push gate this refines), ADR-0001 §4
  (embedding-free hot path — preserved), ADR-0006 (deferred score-banding), exp-0622
  (the trap this addresses), #0067 (the telemetry that yields the labeled volume needed
  to validate this and to reconsider calibration), #0005 (the precision eval that gates
  the rollout). Once validated it supersedes the *stoplist component* of ADR-0015;
  ADR-0015's df-structural-signal insight stands.

## Context

The push gate (ADR-0015) injects only when a query carries a *discriminative* token.
The shipped definition of "discriminative" is **local document frequency** — a token
present in `1..pushMaxDF` validated records — plus two hand-tuned guards added after
exp-0622: a **fixed `pushMaxDF`** (a small constant, not a fraction of the corpus, so
growth can't loosen the gate) and a **hand-maintained common-word stoplist** (`http`,
`cache`, `request`, …).

exp-0622's root cause is structural and unfixed by those guards: **document frequency
in a small curated corpus is not term specificity**. Common dev vocabulary has low df
only because the corpus is tiny — so an off-domain dev prompt (Svelte + FastAPI)
matched and surfaced Go/SQLite/MCP cards. The stoplist patches the *symptom* (it lists
the specific common words seen so far), but:

- It is **hand-maintained** — every newly-observed common word that leaks is a manual
  addition; the list is never provably complete.
- It needs **re-validation as the corpus grows** — the df boundary is one document wide
  at small N (ADR-0015), so `pushMaxDF` and the stoplist must be re-confirmed against
  the precision/recall sets each corpus generation.

We want a **principled, maintenance-free specificity signal**: one that answers "is this
token rare *in the language of code*?" from a stable external reference instead of from
the tiny local corpus.

## Options considered

1. **Status quo — fixed `pushMaxDF` + a hand-maintained stoplist.** Works today
   (off-topic 8/8 → 0 cards on the negative set), but is a hand-tuned guard that grows by
   manual patching and must be re-validated on corpus growth. The maintenance burden and
   incompleteness are the problem. Kept as the *baseline* the replacement must not
   regress against.
2. **Local-corpus IDF.** Derive specificity from the validated corpus's own IDF. This is
   *exactly the exp-0622 bug* — a tiny corpus makes common words look rare. Rejected.
3. **Global IDF from a large external DEV/CODE background corpus (proposed).** Precompute,
   **offline**, a token→IDF table from a StackOverflow/GitHub-scale dev/code corpus and
   use it on the push hot path as a lookup: a token is specific only if it is rare in the
   *language of code*, independent of the local corpus size. The hot path stays lexical
   and embedding-free (ADR-0001 §4) — it is a map lookup.
   - **Make-or-break detail:** the background corpus **must be dev/code, not general
     English.** General English rates `http`/`cache`/`request` as *rare* and would
     **re-introduce the exp-0622 bug**. This is the single most important constraint on
     the implementation.
4. **Score calibration (Platt / isotonic) on the gate score.** Out of scope — see below.

## Decision

Adopt **Option 3** as the direction: a **global dev/code IDF** becomes the push gate's
specificity signal, replacing the hand-maintained stoplist. Phased and reversible:

- **Phase 1 — run alongside.** Land the global-IDF lookup *in addition to* the existing
  stoplist + `pushMaxDF`, gating on both. Add a `TestPushGate*` guard for the global-IDF
  path and keep the live-corpus precision/recall test (the exp-0622 guard) green. Watch
  for **double-filtering** — a genuine query silenced because both signals reject it.
- **Phase 2 — retire the stoplist** only after the global-IDF path proves **no
  regression** against the negative (off-domain) and positive (on-topic) sets *and*
  against #0067's real-traffic labels.
- **Invariants preserved:** embedding-free hot path (ADR-0001 §4), the `k≤3` cap,
  "empty is an answer", and quarantined-never-pushed.

**Explicitly deferred / out of scope (not part of this decision):**

- **Score calibration (Platt / isotonic).** It overfits at tens-of-records and could
  itself breach the `k≤3` cap by pushing borderline queries over the floor (rubber-duck
  validated, #0068). Revisit only once #0067 yields real labeled volume.
- **Wilson confidence intervals** are reported for honesty, not used as a gate at the
  current N.

The **implementation is a follow-up this ADR authorizes**: sourcing + licensing the
dev/code background corpus, the offline IDF precompute, the on-disk/embedded IDF-table
size budget, and the gate wiring. It is intentionally *not* built here — this records the
decision so the build is principled rather than another hand-tuned guess.

## Consequences

- **Removes the maintenance burden and the re-validate-on-growth requirement.** A global
  IDF is corpus-size-independent: it does not drift as the local corpus grows, so the
  per-generation re-confirmation ADR-0015 requires goes away.
- **Concentrates the risk in one place:** the choice of background corpus. A wrong corpus
  (general English) silently re-introduces exp-0622. The Phase-1 run-alongside plus the
  precision eval are the guardrails that catch this before the stoplist is retired.
- **A new offline asset to source and budget.** The IDF table is an external dependency on
  a licensed dev/code dataset (e.g. a StackOverflow dump is CC-BY-SA — attribution +
  share-alike handling) and a footprint decision (embed vs load). The follow-up owns
  these; until it lands, the shipped stoplist + `pushMaxDF` remain the gate.
- **Reversible.** Phase 1 changes nothing the stoplist did not already gate; if the
  global-IDF path regresses, it is removed and the stoplist stands.
- **Supersedes the stoplist component of ADR-0015 only after validation** — not on
  acceptance of this proposal. ADR-0015's core insight (gate on a structural rarity
  signal, not a magnitude floor) is unchanged; this makes the rarity signal principled.
