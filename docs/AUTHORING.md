# AUTHORING — the canon for §5-clean experience records

How to **author** an experience record from a problem we know exists but have no
license-clean text for: independently re-derive the fact, write it in our own
words, prove it with our own executed tests. This is the operational home for the
discipline decided in
[ADR-0011 §5](adr/ADR-0011-corpus-growth-and-validation-engine.md) (the
[decision memo](adr/ADR-0011-section-5-decision-memo.md) carries the legal
reasoning and horia's sign-off) and grounded in
[ADR-0003 §4](adr/ADR-0003-corpus-bootstrap-source-scope.md) ("distill, never
copy"). Record shape is normative in [SCHEMA.md](SCHEMA.md); vocabulary in
[CONTEXT.md](CONTEXT.md). This file is the *procedure*; the *decision* and its
legal theory live in ADR-0011 §5 and the memo — linked, not restated.

> **Scope of the decision.** ADR-0011 §5 is **accepted for the INTERNAL /
> single-tenant corpus only** (horia, 2026-06-23). The **commercial pack stays
> gated** on a real, separate legal review. Authoring is therefore unblocked for
> internal use *today*; nothing here clears a record for a commercial pack.

## When to author (vs. import vs. dogfood-capture)

Three ways a record is born; pick by where the knowledge comes from:

| Path | Source of the fact | `provenance.source_license` | Commercial pack |
|---|---|---|---|
| **Dogfood capture** | a trap that bit *this* repo during our own work | *(empty)* | safe — it is our own work |
| **Import** ([ADR-0003](adr/ADR-0003-corpus-bootstrap-source-scope.md)) | a license-clean source distilled to a fact (OSV, changelog, PEP) | `none (facts only)`, or an SPDX id + `source_url` | safe / per-license |
| **Author** (this doc, [§5](adr/ADR-0011-corpus-growth-and-validation-engine.md)) | a problem *class* we are aware of publicly (Stack Overflow / issues / blogs / model training) but re-derive ourselves | `none (authored, internal-only)` | **excluded until legal review** |

Authoring is the only lever that can *deliberately* cover a domain (the corpus is
starved of engineering traps outside Go — see
[#0088](issues/0088-corpus-coverage-seed-engineering-traps-across-the-full-stack-rn-react-ts-python-native-not-just-security-advisories.md)).
Reach for it when neither dogfood nor a license-clean import will reach a cell in
a reasonable time.

## The rule: topic, never content

Use the public source (and the model's training) **only as awareness that a
problem class exists — the topic, never the content.** Then:

1. **Never** ingest, store, quote, or closely paraphrase third-party text or a
   non-trivial snippet. Never scrape Stack Overflow or use its data dump (so its
   ToS is never even triggered).
2. **Independently re-derive** the fact from first principles + **official docs**
   + **execution**. If you cannot re-derive and execute it, you do not understand
   it well enough to author it — stop.
3. **Author our own** description and **as many original tests as the gotcha
   requires** (positive *and* the dead-ends), written from your re-derivation, not
   from recollection of any post.
4. Record provenance **honestly** as authored — never a false `source_url` /
   "derived from <url>" (we did not derive from it and owe no attribution).

**Why this is clean** is the legal theory of
[ADR-0011 §5](adr/ADR-0011-corpus-growth-and-validation-engine.md) and the
[decision memo](adr/ADR-0011-section-5-decision-memo.md) — their canonical home,
not restated here (facts vs. copyrightable expression, ShareAlike scope, the
executed test as the structural firewall). This canon is the *procedure*; that
ADR is the *decision*. It is not legal advice — which is why the commercial pack
stays gated.

## The procedure: re-derive → author → original tests → execution-validate

1. **State the fact** in one sentence, as a property of the platform/language
   ("a typed nil returned as a Go `error` is a non-nil interface"). Confirm it
   against the official spec/docs.
2. **Author the record** in [SCHEMA.md](SCHEMA.md) shape: `symptom` (what an
   agent observes — often no error string; that is fine, see `exp-0002`),
   `resolution.root_cause` / `fix`, and `resolution.dead_ends` (the part SO never
   captures — what looks like a fix but is not).
3. **Write original tests** as repro scripts under `experience/repro/`, with
   fail-to-pass discipline (`docs/SCHEMA.md` → `guard`): a `positive` that
   reproduces the trap *and* shows the escape, and a `negative` for each tempting
   dead-end that must stay failing. Self-contained, OFFLINE, broker-ready (`/work`
   is the writable + exec-able volume; `TMPDIR=/work` — see `exp-0017`).
4. **Execution-validate** through the real sandbox:
   `twiceshy doctor revalidate -corpus <dir> -json -attest`. The attestation
   (`holds: true`, the matrix labels it `reproduced_under`) is the evidence a
   reviewer reads. **The executed test is the licensing firewall** — an original,
   executed test is structurally ours, not a restatement of a post.

## Provenance: how an authored record is marked

- `provenance.source.author` — the human/agent who authored it (e.g. `claude`),
  **not** `twiceshy-importer`.
- `provenance.source_license: none (authored, internal-only)` — the
  [`record.SourceLicenseAuthoredInternal`](../internal/record/record.go) sentinel.
  It says: no external license obligation, **and** not cleared for commercial use.
- **No `provenance.source_url`** — there is no URL we derived from. Setting one
  would be the dishonest "derived from <url>" provenance §5 forbids; the record
  validator rejects a `source_url` on an authored-internal record.

## Lifecycle: born quarantined → validated

An authored record runs the **same** gates as any other (CONTEXT.md, SCHEMA.md
lifecycle): born `quarantined`; promoted to `validated` only by **execution proof
+ a human PR review**, or autonomously by a **holding attestation + a diverse
judge** ([ADR-0013](adr/ADR-0013-closed-loop-autonomous-validation.md), #0029).
Authoring earns **no** shortcut past quarantine, the judge, or human veto.

## The commercial gate (internal-only, mechanically enforced)

§5 records are cleared for the **internal** corpus only. That is not left to
memory: [`pack.Classify`](../internal/pack/pack.go) maps
`none (authored, internal-only)` to **commercial-ineligible (fail-closed)**, so
`twiceshy pack` keeps them out of a commercial pack while still shipping them in
the internal pack — the same build-time check ADR-0003 §4 uses for copyleft. A
commercial pack ships these only after a real legal review changes that policy.

## Worked example

[`exp-2753`](../experience/2026/2753-go-typed-nil-interface-not-nil.md) — "a nil
pointer returned as a Go error is not nil". Topic widely known publicly; the
description and both tests re-derived from the Go spec and written from scratch.
It carries the §5 sentinel and no `source_url`, ships its positive + negative
repros (`experience/repro/2753-go-typed-nil-*.sh`), and **execution-validates
green** under the real broker (`holds: true`, `reproduced_under: [go1.25]`).

## Residual risk + mitigations

The real danger is **not** the licensing theory — it is **near-verbatim
reproduction**: a model emitting a memorized snippet/phrase while "authoring".
That is a behavior, not a rule, so it cannot be fully mechanized. Defense in
depth:

1. **Author-from-spec discipline** (this canon; ADR-0003 §4) — author from the
   re-derived fact + official docs + the test, never from recollection of a post.
   Needs human care on review; it is the primary control.
2. **The executed-test requirement** — prose alone cannot pass the validation
   engine; an original test is structurally ours.
3. **The PR + judge + human-veto gates** already in place.
4. *Not yet built:* an optional **similarity check** that flags a draft whose
   text is suspiciously close to a known public snippet — tracked as a follow-up,
   an extra net, never the primary control.

## Checklist (per authored record)

- [ ] The fact is re-derived from first principles + official docs; no third-party
      text/snippet ingested, quoted, or paraphrased.
- [ ] `symptom` / `resolution` / `dead_ends` written in our own words.
- [ ] Original `positive` repro (trap + escape) and a `negative` repro per dead-end.
- [ ] `provenance.source_license: none (authored, internal-only)`, author set, **no** `source_url`.
- [ ] `twiceshy doctor revalidate` attests `holds: true`.
- [ ] Born `quarantined`; promotion only via execution proof + human/judge gate.

## Not yet built (follow-ups)

- **Similarity check** (near-verbatim flagger) —
  [#0090](issues/0090-authored-record-similarity-check-flag-near-verbatim-reproduction-of-public-snippets-adr-0011-5-mitigation.md);
  the §5 residual-risk mitigation, an extra net.
- **Authoring-scaffold CLI** (`twiceshy author …`) to pre-stage a record + repro
  skeleton —
  [#0091](issues/0091-authoring-scaffold-cli-twiceshy-author-pre-stages-a-record-repro-skeleton.md);
  convenience; the path above works without it.
