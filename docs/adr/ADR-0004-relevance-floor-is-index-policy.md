# ADR-0004: The relevance floor is index policy, not a per-call argument

- **Status:** Accepted (2026-06-17)
- **Deciders:** horia
- **Related:** [ADR-0001 §3–4](ADR-0001-architecture.md) (retrieval
  precedence, hard cap, relevance floor, embedding-free hot path — **locked**);
  [ADR-0006](ADR-0006-defer-score-banding.md) (three-state novelty keys on this
  floor; score-banding deferred); the seam this touches is registered in
  [CONTRACTS.md](../CONTRACTS.md) per [ADR-0005](ADR-0005-stable-seams.md).

## Context

[ADR-0001 §3](ADR-0001-architecture.md) locks the **relevance floor** as an
invariant: a score threshold below which *nothing* is returned for injection,
because **near-miss** injection is the measured #1 failure mode
([CONTEXT.md](../CONTEXT.md) "near-miss"). §1.121 is explicit: "The relevance
floor and k≤3 cap are invariants, not tunables to disable."

The Phase-3 write path violated this in practice. The floor is carried on
`index.Query.Floor` (a `float64`), and `index.Search` treats it as a pure
mechanism: `Floor > 0` applies the floor, `Floor <= 0` disables it. But the
field's zero value is "disabled", and the two callers that classify novelty
construct their queries without setting it:

- `index.Assess` runs `Search` and maps the result to `known` / `similar` /
  `novel` ([CONTEXT.md] novelty, [ADR-0006]).
- `ingest.Prepare` calls `Assess` for each dedup probe with
  `index.Query{Text: probe, Repo: repo}` — `Floor` unset.

With `Floor == 0`, every probe runs floor-off, so *any* lexical overlap clears
the (absent) floor and becomes `similar`. **`novel` can never fire for a
draft that shares even one common token with the corpus.** The locked
invariant degraded into a per-call accident: present only when a caller
happened to remember to pass `Floor`, absent on the one code path
(dedup-at-ingest) that most needs it.

The fix is to make the floor **policy of the index layer**, applied by default,
overridable only by an explicit, greppable opt-out — never silently absent
because a struct field defaulted to zero.

## Decision

1. **`index.DefaultFloor` is the policy, declared beside `MaxK`.** A new
   exported constant `DefaultFloor float64` sits next to `MaxK` in
   `internal/index`, with the same status: a locked invariant made concrete,
   not a tunable. It is the floor every novelty classification uses unless a
   caller explicitly opts out.

2. **`Assess` applies `DefaultFloor`; `Search` stays the pure mechanism.**
   The policy lives in exactly one place — the assessment layer. `Assess`
   substitutes the default when the caller left the floor unset, then calls
   `Search`. `Search` is unchanged: `Floor > 0` applies the floor, `Floor <= 0`
   does not. This keeps the split clean — `Search` is a mechanism that does
   what it is told; `Assess`/`Prepare` is the policy that decides what to tell
   it. Because `Prepare` reaches the index only through `Assess`, the
   dedup-at-ingest path inherits the floor automatically; the bug class
   ("a caller forgot to pass `Floor`") becomes unrepresentable.

3. **Floor-off is expressed by `index.FloorOff`, a named sentinel.** Disabling
   the floor is a real need for tests and for future diagnostic/pull callers
   that want raw recall. It must read as intent, not as an accidental zero, so
   it gets an exported negative constant `FloorOff`. `Assess` treats
   `Floor == 0` (the zero value) as "apply `DefaultFloor`" and any explicit
   `Floor` — positive (a specific threshold) or `FloorOff` (off) — as the
   caller's override. Direct `Search` callers are unaffected: `0` still means
   off at the mechanism layer.

4. **`DefaultFloor`'s literal value is pinned empirically, with a guarding
   test.** BM25 scores are corpus-dependent and unbounded, so the constant is a
   *coarse* threshold whose only job in this phase is to demote a single weak
   token while a genuine multi-term match survives. It is chosen against the
   seed corpus and locked by a test that asserts exactly that boundary (the
   `index` vs. multi-term case already in `assess_test.go`). Principled,
   normalized score-banding is explicitly **out of scope** and deferred to the
   dense/RRF retrieval phase — see [ADR-0006](ADR-0006-defer-score-banding.md).

## Options

- **A — `float64` + named sentinels (chosen).** Keep `Query.Floor float64`; add
  `DefaultFloor` and `FloorOff` constants; `Assess` maps the zero value to
  `DefaultFloor`. Scalars stay scalars (idiomatic Go), the diff is minimal,
  `Search`'s contract is untouched, and both "off" and "the policy default" are
  named, greppable constants. Cost: the zero value means "apply default", not
  "off", at the `Assess` layer — a deliberate inversion that callers never
  meaningfully relied on (zero *was* the latent bug).

- **B — optional `*float64`.** Make `Query.Floor` a pointer: `nil` → apply
  `DefaultFloor`, non-`nil` → exact value (so `&0.0` is explicit-off). Three
  states are first-class and unambiguous. Rejected: pointer-for-scalar adds
  plumbing at every `Search` call site for a knob that is set rarely, and the
  named-sentinel approach already makes intent explicit without it. If a future
  caller genuinely needs to distinguish "unset" from "explicit zero" at the
  `Search` layer, this is the one-line supersede to make.

- **C — apply `DefaultFloor` in each caller (`Prepare`, push-channel handlers).**
  Leave `Search`/`Assess` mechanical; have every query-constructing caller set
  `Floor: DefaultFloor`. Rejected outright: this is exactly the per-call
  accident this ADR exists to remove — it reintroduces the original bug the
  moment a new caller forgets the line.

## Consequences

- The floor becomes structural: any code path that classifies novelty enforces
  it by default, satisfying the [ADR-0001 §3](ADR-0001-architecture.md) locked
  invariant on the write path, not just the read path.
- `ingest.Prepare` gains real `novel` outcomes — a draft that shares only a weak
  token with the corpus is now correctly classified `novel`, so genuinely new
  experience is captured instead of being mislabeled `similar`.
- Tests that exercised floor-off behavior through `Assess` must say so
  explicitly with `Floor: index.FloorOff`; an unset `Floor` now means "apply
  the policy default" there. `Search`-level tests are unaffected (`0` is still
  off at the mechanism). This is the test churn item-2 lands.
- `DefaultFloor` is a coarse, corpus-sensitive constant by admission; its value
  is owned by a guarding test and revisited when [ADR-0006]'s banding lands. It
  is not a runtime tunable and is not exposed as a server flag.
- The implementation (the constant, the `Assess` change, the test updates) is a
  separate, test-first change tracked as the next roadmap item; this ADR records
  only the decision.
</content>
</invoke>
