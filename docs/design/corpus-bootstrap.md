# Design: bootstrapping the experience corpus from public version-knowledge

- **Status:** Decided (2026-06-13) — source scope resolved as **option A**
  (license-clean only), seeded with the option-C subset first; graduated to
  [ADR-0003](../adr/ADR-0003-corpus-bootstrap-source-scope.md). This doc is
  retained as the rationale of record.
- **Date:** 2026-06-13
- **Deciders:** horia
- **Grounding:** two verified research fan-outs (2026-06-13), companion to
  [PLATFORM_RESEARCH.md](../research/PLATFORM_RESEARCH.md) and
  [ADR-0002](../adr/ADR-0002-licensing-strategy.md). Primary-source citations
  inline; load-bearing licensing claims were verified against the CC BY-SA
  legal code, the GHSA/OSV/endoflife license files, and US copyright case law.

## TL;DR

Yes, bootstrap the corpus from existing public knowledge about recent
framework/language versions — but the research **inverts the naive premise**.
A Stack Overflow scrape is the worst starting point on both axes that matter
here; the best sources (codemods, GitHub Advisory Database, OSV,
endoflife.date) are structured, license-clean, and in two cases yield
*executable* guards. Two hard gates from twiceshy's own locked architecture
shape everything below:

1. **Licensing** — share-alike attaches to copyrightable *expression*, not to
   *facts*. Ingesting the fact "API `X` was removed in v5, use `Y`" is clean;
   pasting a Stack Overflow answer's prose or a non-trivial snippet inherits
   CC BY-SA and would destroy the commercial-pack option in ADR-0002.
2. **Validation** — a record only becomes `validated` with a fail-to-pass
   guard; everything else is born `quarantined` and never reaches the push
   channel. A bulk import is, by construction, a pile of pull-only quarantined
   records — and the prior art says that is *correct*: low-precision corpora
   measurably mislead agents.

Net recommendation: a **precision-first importer** that emits quarantined
records from license-clean structured sources, plus two additive schema fields
(`source_license`, `source_url`) so the pack builder can mechanically keep
commercial packs clean. This widens Phase 0 (issue #1) from our own repos to
external clean sources; promotion to `validated` still waits on Doctor 3
(Phase 4).

---

## 1. The two gates (why this is not a scrape)

### Gate 1 — licensing (the make-or-break for ADR-0002)

The durable value is a corpus we can license *separately* from the AGPL code,
possibly commercially (ADR-0002 §4). Any ingested content carrying a
**share-alike / copyleft** obligation would "infect" the corpus and foreclose
that. The decisive, verified finding:

> **CC BY-SA's share-alike clause attaches only to "Adapted Material" —
> material derived from the original "in a manner requiring permission under
> Copyright."** ([CC BY-SA 4.0 legal code §1, §3(b)](https://creativecommons.org/licenses/by-sa/4.0/legalcode.en))

And facts require no permission: *Feist v. Rural* (1991) — facts and
discoveries are not copyrightable, only original expression and original
selection/arrangement; *Google v. Oracle* (2021) — functional material (API
declarations) gets only "thin" protection
([Supreme Court](https://supreme.justia.com/cases/federal/us/593/18-956/)).
So "React 16.3 deprecated `componentWillMount`; use `componentDidMount`" is a
fact — distilling it creates no Adapted Material, incurs no share-alike, needs
no attribution. Copying the answer's *prose* or a *snippet* does.

**The clean/dirty line is therefore editorial, not source-based.** You may
*read* Stack Overflow while curating; you may not *copy* it into a
distributable commercial pack.

One honest caveat: the facts-vs-expression line is principled but
fact-intensive at the margin — a large, systematically-selected *set* of facts
can attract "thin" protection on its selection/arrangement. The safe
engineering posture is **short, fact-shaped, independently-authored records**,
never close paraphrase.

### Gate 2 — validation (twiceshy's own lifecycle)

Per [SCHEMA.md](../SCHEMA.md), `quarantined → validated` requires a sandbox
fail-to-pass guard **plus** human review; quarantined records never enter the
push channel. Therefore **everything imported is born `quarantined` and is
pull-only** until a guard exists and Doctor 3 (Phase 4) can run it. This is
safe and on-architecture — it just means the near-term payoff of importing is
a richer *pull* channel, not push-channel trap cards. The prior art validates
the conservatism: GitChameleon 2.0 shows frontier models reach only 48–51% on
version-specific tasks, and RAG over low-precision corpora (raw SO,
string-match datasets) actively misleads
([arXiv 2507.12367](https://arxiv.org/abs/2507.12367)).

---

## 2. Source scope — risks & benefits (DECIDED — option A, seeded with C)

> **Decision (2026-06-13, → [ADR-0003](../adr/ADR-0003-corpus-bootstrap-source-scope.md)):**
> **option A (license-clean only)**, seeded with the option-C subset first.
> Stack Overflow (option B) is deferred — admitted later only if a concrete
> coverage gap appears, and only behind the mechanically-enforced facts-only
> rule (§4). The trade-off analysis below is retained as the rationale of
> record.

This was the only choice that is hard to reverse once corpus content exists.
Three options, with the trade-offs spelled out:

| | **A. License-clean only** *(recommended)* | **B. Clean + Stack Overflow facts-only** | **C. Highest-precision only** |
|---|---|---|---|
| **Sources** | GHSA, OSV (permissive records), CVE/NVD, endoflife.date, codemods, GitChameleon, official changelogs (facts-only) | A + Stack Overflow, distilled to facts only, `source_url` recorded | Codemods + GitChameleon only |
| **Coverage** | Broad on security/version/breaking-change facts; thin on "tribal" workarounds | Broadest — SO is where undocumented gotchas live | Narrowest — only what ships with an executable transform/test |
| **Licensing risk** | **Lowest.** All permissive/attribution-only; commercial packs stay clean by construction | **Elevated.** Relies on the facts-only rule holding per-record; thin-protection margin on bulk SO; attribution burden; must avoid the gated SO dump (its click-through bans commercial/LLM use) | **Lowest.** Codemods are typically MIT/Apache; GitChameleon is a research dataset (check its license at ingest) |
| **Validation yield** | Mixed — codemods/GitChameleon can reach `validated`; OSV/changelogs mostly `quarantined` | Same as A; SO adds only quarantined records (no repro) | **Highest** — almost everything carries a runnable before/after |
| **Maintenance** | Moderate — structured feeds, stable schemas | **Highest** — SO needs dedup, quality/security filtering, per-record license tracking | **Lowest** — small, self-checking |
| **Commercial-pack safe?** | **Yes** | Yes *if* the facts-only rule is mechanically enforced and audited | **Yes** |

**Recommendation:** start with **A**, and add SO (option B) *later and
narrowly* only if real coverage gaps appear — and only behind the mechanically
enforced facts-only rule (§4). Option C is the right answer if you want the
smallest, highest-signal corpus first and are willing to grow coverage slowly;
it is also the natural *first slice of A*. In practice: **build the importer
for A, seed it with the C subset first** (the records that can reach
`validated`), then add the metadata tiers.

---

## 3. Source tiers & schema mapping

Ranked by twiceshy-fit = (can yield a runnable `guard` → `validated`) ×
(fingerprintable → hot-path) × (precision/freshness).

| Source | What it gives | Maps to | Guard? | Hot-path? | License |
|---|---|---|---|---|---|
| **Codemods** (jscodeshift/react-codemod, `ng update`, `go fix`, `cargo fix`, `pyupgrade`/`ruff`, ast-grep) | executable trap→fix transforms; before/after = a fail-to-pass pair | `fix`/`convention`; `applies_to` (source→target version); transform fixture = `guard.repro` | **Yes — strongest** | the resulting lint/compile error is fingerprintable | usually MIT/Apache |
| **GitChameleon 2.0** ([arXiv 2507.12367](https://arxiv.org/abs/2507.12367)) | 328 Python version-breaking problems, each with hidden+visible tests | `trap`/`fix`; `applies_to{PyPI,pkg,version}`; tests → `guard`+`guarding_test` | **Yes** | once the version-mismatch error is recorded | research dataset (verify at ingest) |
| **OSV / GitHub Advisory DB** (schema v1.7.5) | `affected[].ranges` (`introduced`/`fixed`), `versions[]`, aliases, references | **`applies_to` near 1:1**; `resolution.fix` = fixed version | partial (only if a PoC/FIX commit exists) | CVE/GHSA id is a stable exact key | GHSA = **CC-BY-4.0**; OSV = **per-record** |
| **Deprecation lint** (typescript-eslint `no-deprecated`, staticcheck `SA1019`, `@deprecated`) | deprecated-API diagnostics | `trap`/`convention`; `symptom.error_signatures` = the lint message | partial (minimal failing fixture) | **yes — diagnostic strings normalize → fingerprint** | tool-dependent (permissive) |
| **endoflife.date** | EOL/support windows per release cycle | `provenance.valid.until` / D2 staleness — **not content** | no | no | **MIT** |
| **Official release notes / PEPs / migration guides** | prose breaking-change/deprecation lists | `trap`; `symptom.summary`; `applies_to` | rarely (unless paired with a codemod) | no (until an error sig is attached) | facts: none; prose: copyrighted |
| **Renovate / Dependabot** | version-bump *events* | trigger for D2/D3 re-validation — **not content** | n/a | n/a | n/a |

License verdicts verified primary:
[GHSA = CC-BY-4.0](https://github.com/github/advisory-database/blob/main/LICENSE.md),
[OSV per-source](https://google.github.io/osv.dev/data/),
[endoflife.date = MIT](https://github.com/endoflife-date/endoflife.date/blob/master/LICENSE),
[CVE ToU](https://www.cve.org), NVD = US-gov public domain (17 U.S.C. §105).
Stack Overflow = [CC BY-SA 4.0](https://stackoverflow.blog/cc-by-sa/), and its
[official data dump is gated](https://stackoverflow.co/data-licensing/) behind
a click-through banning commercial/LLM use — quarantine/reference-only.

**Prior-art reality check** (why precision-first, not volume): GitChameleon
(execution-tested, 48–51% even for frontier models) and VersiCode
([arXiv 2406.07411](https://arxiv.org/abs/2406.07411), 300 libs / 2000+
versions but only string-match labels) both show version-specificity is hard
and that *executable tests are the only reliable signal*. ReCode/“deprecated
API” studies (ICSE 2025) find correct docs improve unseen-API success only
~13.5% and don't guarantee adherence — i.e. the **runnable guard beats the
prose**, exactly twiceshy's bet. [Context7](https://context7.com) (MIT, ~104k
libraries) is the closest production analogue but serves *docs*, not validated
traps with repros — complementary, not competing.

---

## 4. The licensing rule (normative) + the schema addition

**The one rule that keeps the corpus clean:**

> A record may contain only distilled **facts**, authored in twiceshy's own
> words — never third-party expression or non-trivial code — **unless** the
> source license is permissive (public-domain / CC0 / CC-BY / MIT / Apache)
> *and* attribution is recorded. This single rule severs CC BY-SA share-alike
> and preserves the commercial-pack option.

To make that rule *mechanically enforceable* (not just a guideline), add two
optional fields to `provenance`, mirroring OSV's own per-source-license model:

```yaml
provenance:
  source: { author: "twiceshy-importer", session: null, pr: null }
  source_license: "CC-BY-4.0"        # SPDX id, or "none (facts only)"
  source_url: "https://github.com/advisories/GHSA-..."
  # ...existing fields...
```

- Both fields are **optional and additive**, so existing records and the
  current validator's other rules are unaffected; this stays `schema_version: 1`
  (adding optional fields is non-breaking — old records still validate; only
  the JSON Schema's `additionalProperties: false` on `provenance` and
  `internal/record` need extending). A `make ci` schema test should assert the
  new fields are optional and SPDX-shaped.
- The **pack builder** then mechanically excludes any record whose
  `source_license` is copyleft/contract-encumbered from commercial packs, and
  emits attribution for CC-BY records — turning ADR-0002's licensing intent
  into a build-time check instead of a manual audit.

---

## 5. The bootstrap pipeline

1. **Ingest order, precision-first:** codemods → GitChameleon → maintainer
   fix-tools (`ng update`/`go fix`/`cargo fix`/`pyupgrade`) → OSV/GHSA →
   deprecation-lint → release-note facts. endoflife.date + Renovate are
   sidecars, never content.
2. **`validated`-capable tier** (carries a runnable guard): codemod
   before/after fixtures, GitChameleon visible+hidden tests, tool-transform
   diffs that compile+test clean. Guard auto-wrapped as a fail-to-pass repro;
   `kind=fix`/`convention`. *Still lands `quarantined` until Doctor 3 runs the
   guard (Phase 4) — but it is promotion-ready.*
3. **`quarantined`/reference tier:** OSV without a PoC, string-match datasets,
   synthetic updates, raw release-note facts. Pull-only; never pushed.
4. **`applies_to`:** take OSV ranges verbatim
   (`{ecosystem, package, introduced, fixed}`); for codemods/tools, derive the
   range from the migration's source→target majors.
5. **Fingerprints:** compute over normalized lint/compiler/runtime **error
   strings only** (deprecation diagnostics, compiler errors, CVE/GHSA aliases).
   Codemod- and doc-only records are pull-only — **no fabricated fingerprints**.
6. **Provenance:** `valid.from` = source release date; `valid.until` from
   endoflife.date EOL (feeds D2 staleness); `source_license`/`source_url` per §4.
7. **Re-validation:** Renovate/Dependabot bumps trigger D2/D3 — re-run guards
   whose `applies_to` now includes the new version; demote to `quarantined` if
   the repro stops failing pre-fix.
8. **Near-miss discipline (the invariant):** dedupe by `fingerprint` +
   `applies_to`; cap one record per `(package, breaking-change)`; keep
   prose/string-match sources **below the relevance floor and pull-only**, so a
   weak match injects *nothing* rather than flooding the k≤3 budget. Importing
   must never trade precision for volume.
9. **Dogfood:** each promotion to `validated` ships its guard test, per the
   repo's hard rule.

---

## 6. Sequencing & relationship to the roadmap

- **Extends Phase 0 / issue #1** (seed corpus) from our own repos to external
  license-clean sources. The Phase 1 read path already indexes whatever lands
  in `experience/`.
- **New component:** an `importer` — either a `twiceshy ingest <source>`
  subcommand or a sixth doctor ("D6 importer") that opens a PR of quarantined
  records, same trust boundary as the write path (issue #3).
- **Honest caveat:** until Doctor 3 (Phase 4, issue #4) exists to run guards in
  a sandbox, **all imported records stay `quarantined` → pull-only → never push
  channel.** The importer can land any time to grow the pull corpus; promotion
  is gated on D3. No invariant is bent.
- **Touches:** ADR-0001 §2 (schema fields), §6 (quarantine/trust boundary), §7
  (doctors); ADR-0002 §4 (pack licensing, now build-enforced). None is broken;
  this doc should graduate to an ADR once §2's scope is chosen.

---

## 7. Proof-of-concept records

Two illustrative records live in [`examples/`](examples/), drawn from the tier
that is license-clean under **all** scope options, so they pre-commit nothing.
They use the *proposed* schema (the `source_license`/`source_url` fields of §4)
and are intentionally **not** under `experience/`, so they are documentation,
not yet-blessed corpus, and do not run through the validator/indexer.

- [`examples/exp-poc-go-ioutil-deprecation.md`](examples/exp-poc-go-ioutil-deprecation.md)
  — a `fix` derived from a **maintainer deprecation + tool-assisted rewrite**
  (Go 1.16 `io/ioutil` → `os`/`io`), with a real fingerprintable lint signature
  (`staticcheck SA1019`) and an executable fail-to-pass guard
  ([`examples/repro-go-ioutil-deprecation.sh`](examples/repro-go-ioutil-deprecation.sh)).
  This is the **`validated`-capable, hot-path-eligible** shape.
- [`examples/exp-poc-ghsa-log4shell.md`](examples/exp-poc-ghsa-log4shell.md) —
  a `trap` derived from the **GitHub Advisory Database** (CC-BY-4.0), showing
  the near-1:1 OSV→`applies_to` mapping and the **`quarantined`, pull-only**
  metadata tier (no auto-synthesizable guard).

---

## 8. Decision & recommendation

- **Decided (2026-06-13):** source scope — §2 → **option A (license-clean
  only), seeded with the option-C subset first**
  ([ADR-0003](../adr/ADR-0003-corpus-bootstrap-source-scope.md)). Stack
  Overflow (option B) is deferred — added only later, narrowly, and only behind
  the mechanically enforced facts-only rule.
- **Recommended now (low-risk, proceed unless you object):** adopt the §4
  licensing rule + the two additive provenance fields; build the importer for
  the codemod/GitChameleon/GHSA tiers; keep everything `quarantined` until D3.

---

## Appendix: importer issue draft

> **Title:** Corpus importer: bootstrap quarantined records from license-clean version-knowledge sources
>
> **Body:**
> Bootstrap the corpus (extends Phase 0 / #1) from external **license-clean**
> sources, emitting `quarantined` records via PR (same trust boundary as #3).
> Design: [docs/design/corpus-bootstrap.md](docs/design/corpus-bootstrap.md).
>
> Scope (pending the source-scope decision in the design doc §2; default =
> license-clean only):
> - [ ] Add additive `provenance.source_license` (SPDX) + `source_url` fields;
>   extend `schema/experience-record.v1.schema.json` + `internal/record`;
>   `make ci` asserts they are optional and SPDX-shaped. (ADR-0001 §2)
> - [ ] `twiceshy ingest` (or D6 doctor): emit one quarantined record per
>   `(package, breaking-change)`, deduped by `fingerprint`+`applies_to`. (ADR-0001 §6)
> - [ ] **Codemod adapter** (highest yield): wrap a codemod's before/after as a
>   fail-to-pass `guard.repro`; `kind=fix`/`convention`; derive `applies_to`
>   from source→target majors. Promotion still waits on D3 (#4).
> - [ ] **GitChameleon adapter:** map visible/hidden tests → `guard`/`guarding_test`.
> - [ ] **OSV/GHSA adapter:** map `affected[].ranges` → `applies_to`; record
>   per-source license; quarantined unless a PoC/FIX yields a guard.
> - [ ] **endoflife.date** sidecar → `provenance.valid.until` (feeds D2).
> - [ ] Pack-builder exclusion: drop copyleft/contract-encumbered
>   `source_license` records from commercial packs; emit CC-BY attribution. (ADR-0002 §4)
> - [ ] Near-miss guard: imported prose/string-match records stay pull-only and
>   below the relevance floor.
>
> **Sequencing:** importer grows the pull corpus immediately; nothing reaches
> the push channel until D3 (#4) can run guards. No invariant bent.
