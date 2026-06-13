# ADR-0003: Corpus bootstrap source scope — license-clean only, seeded precision-first

- **Status:** Accepted (2026-06-13)
- **Deciders:** horia
- **Grounding:** [docs/design/corpus-bootstrap.md](../design/corpus-bootstrap.md)
  — the rationale of record, built on two verified research fan-outs
  (2026-06-13) and load-bearing licensing claims checked against the CC BY-SA
  legal code, GHSA/OSV/endoflife license files, and US copyright case law.
- **Related:** [ADR-0001 §2, §6, §7](ADR-0001-architecture.md) (schema,
  quarantine/trust boundary, doctors); [ADR-0002 §4](ADR-0002-licensing-strategy.md)
  (separately licensed packs); tracked issue: corpus importer (#7).

## Context

The durable value of twiceshy is the validated experience corpus, and we want
to bootstrap it from existing public knowledge about recent framework/language
versions rather than wait for it to accrue organically. Two locked constraints
dominate how we may do this:

1. **Licensing (ADR-0002, hard to reverse).** The commercial-pack option
   requires the corpus stay license-clean. Share-alike attaches to copyrightable
   *expression*, not to *facts* (*Feist*; *Google v. Oracle*), so distilling the
   fact "API `X` was removed in v5, use `Y`" is clean, but copying Stack
   Overflow prose or a non-trivial snippet inherits CC BY-SA and forecloses
   commercial packs. Once dirty content exists, this is irreversible.
2. **Validation lifecycle (ADR-0001 §6).** Every imported record is born
   `quarantined` and is pull-only until Doctor 3 (Phase 4, #4) can run its guard
   in a sandbox. Source choice therefore only affects the *pull* channel in the
   near term; nothing reaches the push channel regardless of source.

The corpus-bootstrap design doc framed three source-scope options: **A** —
license-clean only; **B** — A plus Stack Overflow facts-only; **C** —
highest-precision only (codemods + GitChameleon). This ADR records the choice.

## Decision

1. **Source scope is option A: license-clean only.** Admitted sources are
   codemods (MIT/Apache), GitChameleon (research dataset, license verified at
   ingest), OSV / GitHub Advisory Database (per-source / CC-BY-4.0), CVE/NVD
   (public domain), endoflife.date (MIT), and official changelogs / release
   notes / PEPs **as distilled facts only** — never copied prose.

2. **Seed precision-first with the option-C subset.** Ship the codemod and
   GitChameleon adapters first: they are the records that can reach `validated`
   once D3 (#4) lands, carry a runnable before/after guard, and impose the least
   licensing and maintenance burden. Then widen to the rest of A — OSV/GHSA
   metadata (near-1:1 `applies_to` mapping), the endoflife.date sidecar
   (`provenance.valid.until`, feeds D2), and changelog facts.

3. **Stack Overflow (option B) is excluded for now.** It carries the most
   licensing risk (per-record facts-only rule must hold; bulk selection can
   attract "thin" protection; the official data dump's click-through bans
   commercial/LLM use) and the most maintenance, to add content that is
   quarantined, repro-less, pull-only, and can never reach `validated` — exactly
   the low-precision pull corpus the prior art shows misleads agents. It may be
   admitted later, narrowly, only on a demonstrated coverage gap, and only
   behind the mechanically-enforced facts-only rule below.

4. **The licensing rule is normative and mechanically enforced.** A record may
   contain only distilled facts in twiceshy's own words — never third-party
   expression or non-trivial code — unless the source license is permissive
   (public-domain / CC0 / CC-BY / MIT / Apache) and attribution is recorded. Two
   additive optional fields are added to `provenance`: `source_license` (SPDX id,
   or `"none (facts only)"`) and `source_url`. The pack builder mechanically
   excludes copyleft/contract-encumbered records from commercial packs and emits
   attribution for CC-BY records, turning ADR-0002's intent into a build-time
   check.

5. **No invariant is bent.** Everything imported is born `quarantined` →
   pull-only; promotion to `validated` still waits on D3's sandbox fail-to-pass
   guard. The importer grows the pull corpus immediately; the push channel is
   untouched.

## Consequences

- The schema gains two optional, additive `provenance` fields; this stays
  `schema_version: 1` (old records still validate). `make ci` must assert they
  are optional and SPDX-shaped; `internal/record` and the JSON Schema's
  `additionalProperties: false` on `provenance` are extended accordingly.
- The importer (#7) extends Phase 0 (#1) from our own repos to external
  license-clean sources; build for A, but the first shipping slice is the C
  adapters.
- Commercial-pack cleanliness becomes a build-time guarantee, not a manual
  audit — provided the facts-only sources keep editorial discipline (distill,
  never paraphrase-copy), which the per-record `source_license` field cannot
  fully mechanize.
- Coverage of "tribal"/undocumented workarounds stays thin until and unless
  option B is revisited; that is an accepted, reversible trade for clean,
  high-precision provenance.
