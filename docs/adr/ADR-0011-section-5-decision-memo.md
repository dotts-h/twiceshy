# Decision memo — ADR-0011 §5 sign-off (the authoring licensing reframe)

- **For:** horia (the only party who can ratify this — it is a licensing/product call)
- **From:** claude
- **Date:** 2026-06-23
- **Re:** [ADR-0011 §5](ADR-0011-corpus-growth-and-validation-engine.md) — "Licensing
  reframe (PROPOSED, needs Horia's explicit sign-off)"; unblocks
  [#0024](../issues/0024-llm-wrong-canon-so-reframe-authoring-gated-on-adr-0011-section-5-sign-off.md)
  (authoring) and is the critical path for
  [#0088](../issues/0088-corpus-coverage-seed-engineering-traps-across-the-full-stack-rn-react-ts-python-native-not-just-security-advisories.md)
  (corpus coverage).
- **This is not legal advice.** It is engineering decision-support so the call can be
  made deliberately. A real legal review still gates any *commercial* pack (unchanged).

## The decision requested

**Accept ADR-0011 §5 for the INTERNAL / single-tenant corpus only.** Keep the commercial
pack gated on a separate, real legal review. That single scoped "yes" unblocks #0024 and
turns corpus seeding across your stack from *months* (organic) into *weeks* (authored).

**Recommendation: accept, internal-only.** The legal theory is sound, the residual risk
is real but bounded and mitigated, the scope is reversible in practice for internal use,
and the upside is the entire reason twiceshy exists.

## Why this is the gate that matters

The pipeline is built and hardened. The corpus is **not**: 2,757 records, but ~2,700 are
imported security advisories (Dependabot/OSV noise). The records that actually help an
agent build — engineering traps — number a few dozen, **almost entirely Go**. React: 0.
React Native: 0. TS frontend: 0. iOS/Android/macOS/Windows: 0. A session on any of your
non-Go stack cells has nothing to retrieve (proven by a real RN field session that was
write-only).

There are only two ways to fill that gap: **organic dogfood capture** (slow, and only
covers what sessions happen to hit) and **authoring (#0024)**. Authoring is the only lever
that can deliberately cover a domain — and it is blocked solely on this sign-off.

## What §5 actually permits (and forbids)

The rule is narrow and is **"topic, never content"**:

- Stack Overflow / issue-tracker / blog **text stays fully excluded** (CC-BY-SA / ToS).
  Never ingested, stored, quoted, or closely paraphrased. SO is never scraped or its data
  dump used, so **SO's ToS is never even triggered**.
- Those sources (and the model's training) are used **only as awareness that a problem
  class exists** — the topic. For each problem we **independently re-derive the fact** from
  first principles + official docs + execution, and author **our own** description and **our
  own original tests** (as many as the gotcha requires).
- Provenance is recorded honestly as `source = authored+validated`, **not** "derived from
  <url>" — because we did not derive from it and owe no attribution.

## Why it's clean (the legal theory, in plain terms)

- **Facts aren't copyrightable** (*Feist*; idea/expression dichotomy, 17 USC §102(b)). "RN
  pooled events recycle after `await`" is a fact about the platform, not anyone's prose.
- **CC-BY-SA's ShareAlike attaches only to adaptations of the licensed *expression*** —
  which we never make. We don't adapt the post; we re-derive the fact and write our own.
- **The executed test is the structural firewall.** A set of independently-authored,
  *executed* tests that prove fail→pass is our own work product, not a restatement of a
  post — and the validation engine (gVisor matrix, ADR-0011 §3-4) makes that executable
  proof a build requirement, not a claim.

## The residual risk, honestly

The real danger is **not** the licensing theory — it's **near-verbatim reproduction**: an
LLM emitting a memorized snippet/phrase from training while "authoring." This is the one
thing the theory can't fully neutralize, because it's a behavior, not a rule.

Mitigations (defense in depth):
1. **Author-from-spec discipline** (ADR-0003 §4 "distill, never copy") — author from the
   re-derived fact + official docs + the test, not from recollection of a post. Can't be
   fully mechanized → needs human care on review.
2. **The executed-test requirement** — original tests are structurally our own; a record
   that's just prose can't pass the validation engine.
3. **Optional similarity check** — flag a draft whose text is suspiciously close to a known
   public snippet before promotion (cheap to add; an extra net, not the primary control).
4. **The PR + judge + soak gates already in place** — every authored draft is born
   quarantined and runs the full promote/judge/human-veto path.

## Scope and staging (what "yes" does and does not commit to)

| | Internal / single-tenant pack | Commercial pack |
|---|---|---|
| §5 authoring | **Unblocked by this memo** | **Still gated** on a real legal review |
| SO/issue text ingested | Never | Never |
| Provenance | `authored+validated` | `authored+validated` |
| Risk posture | Low; reversible in practice (purge an internal record) | Irreversible (ADR-0002) → needs counsel |

Commercial-pack cleanliness is irreversible (ADR-0002), so the staging is deliberate:
**internal now, commercial later behind counsel.** Accepting internal-only does **not**
pre-commit the commercial decision.

## What changes on sign-off

1. ADR-0011 §5 status → **Accepted (internal/single-tenant)**; ADR-0011 overall can move
   from Proposed toward Accepted for the non-commercial scope.
2. **#0024 unblocks** — the authoring harness (re-derive → author → original tests →
   execution-validate → quarantined draft → promote) can run.
3. The corpus-seeding campaign (#0088) becomes schedulable: a per-domain authoring pass
   (RN, React, TS, Python; Go already partial) using the multi-agent + execution-validation
   machinery, measured by the #0005 eval extended into a per-stack-cell coverage map.

## What does NOT change

- No Stack Overflow / issue-tracker / blog **text** is ever ingested, stored, or quoted.
- Records remain `authored+validated`; no false "derived from <url>" provenance.
- The **commercial pack stays gated** on a separate, real legal review.
- Every authored record still runs the full quarantine → judge → soak → human-veto path.

## The call

- [ ] **Accept §5, internal/single-tenant only** (recommended) — unblocks #0024, keeps the
      commercial gate.
- [ ] **Defer** — keep authoring blocked; corpus coverage stays organic-only (months, and
      patchy). Choose this only if even the internal-only legal posture isn't yet comfortable.
- [ ] **Reject** — abandon authored traps entirely; rely solely on imports + organic
      capture. (Not recommended; it caps the corpus at "advisories + whatever sessions hit.")

Sign-off here (or amending ADR-0011 §5's status line) is all that's needed to start.
