# ADR-0011: Corpus growth as a live feed, with execution-validation as the moat

- **Status:** Proposed overall; **§5 (the licensing reframe) is Accepted for
  INTERNAL / single-tenant use only** (2026-06-23, decider: **horia**) — see the §5
  DECISION note below and the [decision memo](ADR-0011-section-5-decision-memo.md).
  This unblocks #0024 (authoring) at internal scope; the **COMMERCIAL pack stays gated**
  on a real legal review. Phases 1–3 proceed; phase 4 + any commercial pack remain
  gated. (Deciders: **horia** — product direction + the licensing call, his to ratify.)
- **Grounding:** [CORPUS_GROWTH_RESEARCH.md](../research/CORPUS_GROWTH_RESEARCH.md)
  (off-pool research synthesis, 2026-06-18), building on
  [PLATFORM_RESEARCH.md](../research/PLATFORM_RESEARCH.md) and
  [SECURITY_ANALYSIS.md](../research/SECURITY_ANALYSIS.md).
- **Related / extends:** ADR-0001 (architecture), ADR-0002 (licensing),
  ADR-0003 (bootstrap source scope), ADR-0009 (embedding-free hot path),
  ADR-0010 (doctors / D3 deferral). Tracked: #0004 (D3), #0005 (eval), #0007 (importer).
  **Loop closed by [ADR-0013](ADR-0013-closed-loop-autonomous-validation.md)
  (proposed)** — proof + a diverse-model judge auto-promote, and gated outcome
  reports demote/supersede, removing the human from the *provable* loop (epic #0027).

## Context

The corpus is currently a **static seed, not a feed**: importers emit `//go:embed`
curated snapshots; re-running them yields the same records. Validated records are
hand-authored. The differentiated value of twiceshy over an LLM's own weights is
**freshness + execution-validation + negative knowledge** (research §1). To deliver
that, two things must become real: continuous ingestion from live sources, and an
execution-validation engine that makes "validated" *mean* "we ran it and it holds."

## Options considered

- **A — keep stacking embedded/quarantined snapshots.** Cheap; but stays a stale,
  low-trust seed; no moat; doesn't match the design intent (ADR-0003).
- **B — live ingestion only (no validation engine).** A feed, but everything stays
  quarantined/low-trust — same trust ceiling as Context7's unvalidated docs.
- **C — validation engine first, then live ingestion onto it (recommended).** Higher
  upfront cost; but "validated" becomes execution-proven, which is the differentiator
  every prior-art comparison (research §3) points to as unclaimed.

## Decision

1. **Positioning is locked (research §1):** twiceshy is the execution-validated,
   fresh, negative-knowledge "pre-flight landmine check" for coding agents. Every
   build choice serves freshness, validation, or dead-end coverage.

2. **Grow by a live feed, OSV-first (extends ADR-0003).** Build live importers for
   OSV.dev → GHSA → deps.dev → endoflife.date → GitHub Releases/Issues, emitting
   **distilled facts only** (license-clean per ADR-0002/0003; attribution recorded;
   NVD notice where applicable). The embedded YAML stays as the offline seed/fallback.

3. **The validation engine is the moat (Option C; refines PLATFORM_RESEARCH §2).**
   A delta-only, report-only doctor (`internal/repro` / `twiceshy doctor revalidate`)
   runs a record's tests in **gVisor (runsc)** ephemeral containers across a **version
   matrix**, two-phase (prepare=allowlist-egress → execute=`--network=none`), to prove
   fail→pass and **empirically derive `applies_to` version boundaries**. It emits a
   Finding + a signed-able **attestation**; a human flips `validated`/`validated_at` in
   the PR. Promotion is never automatic (git/PR trust boundary, ADR-0001/0008 unchanged).
   **A record carries AS MANY tests as the gotcha requires, not one** — positive tests
   (the fix holds) AND **negative tests that encode the dead-ends** (prove "don't try Z"
   by showing Z still fails), plus variants across inputs/configs/versions. More tests =
   stronger validation, tighter version boundaries, and (see §5) clearer original authorship.

4. **The 3 hardening must-haves are preconditions** for running any untrusted repro
   (research §5, SECURITY_ANALYSIS): broker-enforced ephemeral container with a
   hardcoded policy; a watchdog that guarantees termination+cleanup; the trust boundary
   (only PR-reviewed + ingest-screened repros run — and `internal/screen` must screen
   the **repro script content**, not just record prose). Build none of the engine before
   these.

5. **Licensing reframe — PROPOSED, needs Horia's explicit sign-off (extends ADR-0003 §3).**
   Stack Overflow / issue-tracker / blog **text stays excluded** (CC-BY-SA / ToS, irreversible
   per ADR-0002). But the *problems* they document become admissible under a strict rule:
   - **Use those sources (and the model's training) only as awareness that a problem class
     exists — the topic, never the content.** For each problem, **independently re-derive the
     fact from first principles + official docs + execution**, and author **our own**
     description and **as many original tests as the gotcha requires** (§3). Never ingest,
     store, quote, or closely paraphrase their text or snippets; never scrape SO or use its
     data dump (so SO's ToS is never even triggered).
   - **Why clean:** facts aren't copyrightable (*Feist*; idea/expression, 17 USC §102(b));
     CC-BY-SA's ShareAlike attaches only to adaptations of the licensed *expression*, which we
     never make; and a set of independently-authored, executed tests is structurally our own
     work — not a restatement of someone's post.
   - **Provenance honesty:** these records are `source = authored+validated`, NOT "derived from
     <url>" (we didn't derive from it, and owe no attribution because we didn't use the work).
   - **Residual risk + gate:** the real danger is near-verbatim reproduction of a memorized
     snippet/phrase by the LLM — mitigated by author-from-spec+docs+execution discipline (the
     "distill, never copy" rule of ADR-0003 §4, which can't be fully mechanized → needs human
     care) and an optional similarity check. This ADR is **not legal advice**; because the
     commercial-pack cleanliness (ADR-0002) is irreversible, sign-off should be staged:
     **OK for the internal/single-tenant corpus now; a real legal review gates any COMMERCIAL
     pack shipping SO-derived records.**
   - **DECISION (2026-06-23, horia): §5 is ACCEPTED for INTERNAL / single-tenant use only.**
     #0024 (authoring) is unblocked at internal scope: author the topic, never the content;
     re-derive each fact from first principles + official docs + execution; records stay
     `source = authored+validated`, and no SO / issue-tracker / blog text is ever ingested,
     stored, quoted, or closely paraphrased. The **COMMERCIAL pack remains gated** on a real
     legal review (the ADR-0002 irreversibility is unchanged). Rationale, the legal theory,
     and the residual-risk mitigations are in the [decision memo](ADR-0011-section-5-decision-memo.md).

6. **Prove the value with the eval (#0005).** Build a GitChameleon-style execution
   harness measuring agent task success **with vs without** twiceshy retrieval. This both
   justifies the investment and is the gate that decides whether the (currently deferred,
   dormant) push channel is ever worth re-enabling.

7. **Organic growth via agent/user contributions — through the same gates, and a ToS.**
   Consumers that hit a novel gotcha propose it via `record_experience` (propose-only,
   ADR-0008 — returns a quarantined draft to PR, never a direct write). A contribution is
   **never auto-trusted**: born quarantined, it passes the ingest **screen** (license +
   safety) + the **validation harness** (execution) + human/PR review before `validated`.
   It is also a contamination/poisoning vector → **the service must carry a ToS /
   contributor license grant** (extends ADR-0002's CLA from repo contributors to the hosted
   service): by submitting, the contributor (a) **grants twiceshy a permissive, sublicensable
   license** to the contribution so it can enter commercial packs clean, and (b) **represents
   they have the right to contribute it and that it contains no copied third-party
   text/snippets** (the §5 facts-only/own-words rule applies to them too). Without this, agent
   contributions are a licensing hole — it is a prerequisite for accepting outside contributions
   into a commercial corpus.

8. **Test-generation pipeline + the local-model role (under the execution gate).**
   - Distill the **fact deterministically** from structured sources where possible (e.g.
     OSV metadata → `applies_to` is near-1:1) — *minimize model-generated prose*, since
     generated text carries the only real verbatim-reproduction risk (§5).
   - **Draft tests** with a cheap model (local gpt-oss/qwen-coder, or a frontier executor for
     hard ones) **from the spec + official docs only — never from SO text**, including the
     negative/dead-end tests.
   - The **execution harness is the deterministic gate** that makes a cheap drafter safe: a
     draft that doesn't truly fail-pre / pass-post is auto-rejected. A frontier model + human
     judge the survivors. **The test is the licensing firewall** — an executed, original test
     is structurally ours.
   - **Local LLM = drafter/flagger, never judge** (standing rule): it can draft tests/prose
     and flag suspicious input as a *lead*, but cleanliness/correctness authority stays on
     deterministic checks + execution + frontier/human review. Run Ollama **on-demand per
     ingestion batch** (wake VM 101, then re-park), not 24/7.

9. **Record lifecycle — staleness ≠ deletion.** Records are **version-anchored truths**
   ("X removed in v5" stays true for v5); they lose *relevance*, not *correctness*, as
   versions age. So: **mark + down-rank + supersede, never delete.** endoflife.date (D2)
   marks records whose whole applicable range is EOL as `stale`/low-priority; retrieval
   **ranks by version-match to the querying agent's context** (current queries surface
   current records; an agent on an old version still benefits); the re-validation loop (D3)
   catches genuine correctness drift; `provenance.superseded_by` (already in schema) replaces
   without deleting. Continue building — version-anchoring does most of the work.

## Phasing

1. **Validation harness** (the multiplier; preconditioned on the 3 must-haves) — gVisor
   runner + version matrix + attestation, report-only, one ecosystem (Go or Python) first.
2. **Live OSV/GHSA importer** — feeds Tier-1 facts into the harness.
3. **Deprecations + codemods** (deps.dev / endoflife / changelogs) — most testable, cleanest.
4. **LLM-wrong canon + SO-reframe authoring** (gated on §5 sign-off + the harness).
5. **#0005 eval** runs alongside from phase 1; revisit push only if it earns it.

## Consequences

- New `internal/repro` package + `repro-base` images (pinned by digest) + runsc installed
  on the brain; per-ecosystem fetch proxies for the prepare phase.
- **Schema evolves: `guard.repro` (a single path) → a SET of tests per record** (a tests
  dir / list), positive + negative, so a record can carry as many as the gotcha requires
  (§3). `provenance.security_flags` / `validated_at` already exist; the test-set is the one
  additive schema change (stays `schema_version: 1`-compatible if modeled as an optional list).
- "Validated" gains a precise, auditable meaning (attestation in the promotion PR).
- `internal/ingest` extends from embedded YAML to live fetchers; `internal/screen` extends
  to repro-script content.
- Until §5 is accepted, SO-covered problems remain out of scope; the clause is inert.
- New attack surface (executing untrusted code) is accepted **only** behind the 3 must-haves.
- **Storage is unchanged (ADR-0001) and is the right optimization:** git-backed markdown files
  = source of truth (PR = trust boundary, full provenance), SQLite (FTS5 + fingerprint + optional
  dense BLOBs) = a *derived, disposable* index rebuilt from the files. For a small curated
  corpus this is optimal — **no database-as-truth, no external vector DB** (would be unrequested
  complexity; ADR-0009 already keeps dense pure-Go for the CGO-free build). Future levers
  (premature): incremental rebuild, per-tenant scoping for Tier B.
- **"Irreversible" clarified:** removing a record from the *live* corpus is trivial (delete the
  git file). What's irreversible is having **shipped** dirty content in a *commercial pack* —
  distributed copies can't be recalled and ShareAlike may have attached to the pack. So the
  license gate is **pre-ship**, not post-hoc cleanup.
- **Accepting outside contributions into a commercial corpus requires the Contribution Terms**
  ([docs/legal/CONTRIBUTION_TERMS.md](../legal/CONTRIBUTION_TERMS.md), Decision 7) — a DRAFT
  pending legal review.
