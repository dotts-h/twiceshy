# SCHEMA — the experience record, formally

This is the normative spec for twiceshy experience records (schema
version **1**). The machine-checkable half lives in
[`schema/experience-record.v1.schema.json`](../schema/experience-record.v1.schema.json)
(JSON Schema draft 2020-12, applied to the parsed YAML frontmatter); the
rules that JSON Schema cannot express are listed in
[Cross-field rules](#cross-field-rules) and enforced by the validator
(`internal/record`). Vocabulary: [CONTEXT.md](CONTEXT.md). Rationale:
research [§4](research/EXPERIENCE_SERVICE_RESEARCH.md).

Three fully-worked examples ship in this repo and double as test fixtures:

| id | kind | demonstrates |
|---|---|---|
| [`exp-0001`](../experience/2026/0001-fts5-match-raw-user-input.md) | trap | full shape: signatures, dead-ends, F2P repro script + guarding test |
| [`exp-0002`](../experience/2026/0002-fts5-bm25-negative-scores.md) | trap | a *silent* trap: no error signature, guard without repro |
| [`exp-0003`](../experience/2026/0003-mcp-streamable-http-not-sse.md) | convention | a record with no `symptom` block at all |

## File layout

- One record per file: `experience/YYYY/NNNN-slug.md`, where
  - `YYYY` is the year of `provenance.recorded_at`,
  - `NNNN` is the record's zero-padded number, **monotonically increasing
    repo-wide** (not per-year), matching the `id`,
  - `slug` is kebab-case, derived from the title, stable after creation.
- Repro scripts live in `experience/repro/`, referenced by repo-root-relative
  path from `guard.repro`.
- A file is: YAML frontmatter between `---` fences, then a **non-empty
  markdown narrative body**. Structure is for machines; learning lives in
  the narrative.

## Frontmatter fields

### Top level

| field | type | required | notes |
|---|---|---|---|
| `schema_version` | int | yes | exactly `1` for this spec |
| `id` | string | yes | `^exp-[0-9]{4,}$` |
| `kind` | enum | yes | `trap` \| `fix` \| `dead-end` \| `convention` \| `workflow` |
| `status` | enum | yes | `quarantined` \| `validated` \| `stale` \| `superseded` |
| `title` | string | yes | 8–120 chars; written as the one-line trap card headline |
| `symptom` | object | for `trap`/`fix`/`dead-end` | the retrieval surface |
| `applies_to` | array | no | OSV-style stack fingerprint |
| `resolution` | object | for `trap`/`fix`/`dead-end` | see below |
| `guard` | object | for validated `trap`/`fix` | executable proof |
| `provenance` | object | yes | trust + bi-temporal validity |

### `symptom`

| field | type | required | notes |
|---|---|---|---|
| `summary` | string | yes | free text; what an agent would observe. Embedded later (Phase ≥2); FTS-indexed now |
| `error_signatures` | string[] | no | verbatim-ish error messages; the exact-match surface. The **indexer derives app+generic fingerprints from every signature** |
| `fingerprints` | object | no | *additive* externally sourced fingerprints (e.g. from Sentry): `{app: [sha256:…], generic: [sha256:…]}`. Never required — derived ones come from `error_signatures` |

### `applies_to` (array items)

Each item must carry at least one of `ecosystem`/`package`/`runtime`:

| field | type | notes |
|---|---|---|
| `ecosystem` | string | OSV-style ecosystem name (`Go`, `PyPI`, `npm`, …) or a platform name (`MCP`, `sqlite`) |
| `package` | string | package/module identifier within the ecosystem |
| `versions` | object | `{introduced: string\|null, fixed: string\|null}` — OSV semantics: the record applies from `introduced` until `fixed`; `null` = open bound |
| `runtime` | map[string]string | free-form runtime constraints, e.g. `{sqlite: ">=3.9"}` |

`applies_to` is the **version axis** of bi-temporal validity; it filters and
boosts retrieval and is what Doctor 2 cross-checks against the live world.

### `resolution`

| field | type | required | notes |
|---|---|---|---|
| `root_cause` | string | for `trap`/`fix` | 2–5 contributing factors, not one blame line |
| `fix` | string | for `trap`/`fix` | the change that worked (a trap's *escape*) |
| `dead_ends` | array | for `dead-end` (≥1) | each: `{tried: string, why_it_failed: string}` — the part StackOverflow never captures; allowed (encouraged) on every kind |

### `guard`

| field | type | required | notes |
|---|---|---|---|
| `repro` | string\|null | recommended | repo-root-relative path to a script with **fail-to-pass discipline**: exits non-zero in the trap/pre-fix state, zero post-fix; exit `75` = environment can't run it (skip). Semantically a single **positive** repro; kept for back-compat. Becomes **mandatory for promotion to `validated`** once Doctor 3 ships (Phase 4) |
| `repros` | array | no | optional test-set; each item: `{path, kind, label?}`. `kind`: `positive` (fail-to-pass — the fix holds) or `negative` (dead-end — must stay failing, proves "don't try Z"). `path` uses the same repro-script discipline as `repro`. A record may use `repro`, `repros`, or both; paths within one record's `repros` must be unique. The validation harness (#0020) runs the whole set |
| `guarding_test` | string\|null | see note | the test name that keeps the fix fixed |

A validated `trap`/`fix` requires **executable proof**: a `guarding_test` **or**
a positive repro (`guard.repro`, or a `guard.repros` entry with `kind: positive`).
The repro IS the proof for execution-validated records (ADR-0011, Doctor 3), so it
satisfies the requirement on its own — a record proven by the harness need not
also carry a named Go unit test.

### `provenance`

| field | type | required | notes |
|---|---|---|---|
| `source` | object | yes | `{author: string (required), session: string\|null, pr: string\|null}` |
| `recorded_at` | date | yes | `YYYY-MM-DD` |
| `validated_at` | date\|null | when `status: validated` | last successful sandbox validation |
| `valid` | object | yes | `{from: date (required), until: date\|null}` — the **time axis**; `until` set only by supersession/staleness |
| `source_license` | string | no | SPDX id (e.g. `CC-BY-4.0`, `MIT`) or `none (facts only)`; set by the corpus importer so the pack builder keeps commercial packs license-clean ([ADR-0003](adr/ADR-0003-corpus-bootstrap-source-scope.md) §4) |
| `source_url` | string | no | `http(s)` URL the imported fact was distilled from; recorded alongside `source_license` |
| `security_flags` | string[] | no | hazards the ingestion safety gate detected (e.g. `secret:aws-access-key`, `harmful-code:pipe-to-shell`); set on ingest (#0011). A record with `security_flags` **cannot be `validated`** — it is documented + quarantined |
| `superseded_by` | string\|null | when `status: superseded` | id of the replacement. **Supersede, never delete** |
| `disputes` | string\|null | no | id of an existing record this one is **counter-evidence against** — set on an outcome-report counter-record (#0031). Additive, optional, `exp-NNNN`-shaped like `superseded_by`. It is evidence, not a verdict: #0032 follows it to re-run the original repro plus the counter, and a report never mutates its target |
| `promotion` | object | no | the **audit trail of an autonomous promotion** (#0029, ADR-0013): `{attested_at, reproduced_under?, judge_model, judge_decision}` — the holding attestation and the diverse judge's verdict that flipped this record `quarantined → validated` with no human approver. Additive, optional; set only by the promoter |
| `usage` | object | no | `{retrieved: int, confirmed_helpful: int, last_hit: date\|null}` — Doctor 4's signal; maintained by tooling, zero-valued at creation |

## Lifecycle

```
            (sandbox F2P + human PR review)
quarantined ───────────────────────────────► validated ──► stale
                                                 │
                                                 └────────► superseded
```

- Every agent-proposed record is born `quarantined`. Quarantined records
  never enter the push channel.
- `validated → stale`: Doctor 2/3 demotion (world drifted, repro stopped
  reproducing). Stale records are re-validatable.
- `validated → superseded`: a newer record replaces this one. Set
  `valid.until` and `superseded_by`; the file stays forever.
- There is no delete transition, anywhere.

## Fingerprints (normative algorithm)

A fingerprint is `"sha256:" + hex(sha256(input))`, lowercase. The input is a
domain-separated normalized signature:

- **generic**: `"generic\n" + normalize(signature)`
- **app**: `"app\n" + repo + "\n" + normalize(signature)` where `repo` is the
  originating repository identifier (e.g. `github.com/dotts-h/twiceshy`).

`normalize(s)` applies, in this exact order:

1. lowercase (`strings.ToLower`);
2. replace UUIDs (`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`) with `<uuid>`;
3. replace hex addresses (`0x[0-9a-f]+`) with `<addr>`;
4. replace standalone long hex runs (`\b[0-9a-f]{8,}\b`) with `<hex>`;
5. replace filesystem paths — tokens starting with `/`, `~/`, `./` or `../`
   at the start of the string or after whitespace — with `<path>`
   (identifiers like `modernc.org/sqlite` are deliberately **not** paths);
6. replace standalone digit runs (`\b[0-9]+\b`) with `<num>` — digits
   embedded in identifiers (`fts5`, `utf8`, `sha256`) are discriminative and
   stay;
7. collapse whitespace runs to a single space and trim.

The same algorithm runs at index time (over each `error_signatures` entry)
and at query time (over incoming error text), so fingerprint-exact matching
is a hash-equality lookup. Reference implementation:
`internal/fingerprint`.

## Cross-field rules (validator-enforced, beyond JSON Schema)

1. The filename's `NNNN` must equal the numeric part of `id`; the `YYYY`
   directory must equal the year of `provenance.recorded_at`.
2. The narrative body must be non-empty.
3. `provenance.valid.from` ≤ `valid.until` when both set; `recorded_at` ≤
   `validated_at` when both set.
4. `superseded_by`, when set, must reference an existing record id
   (corpus-level check; skipped in single-file validation).
5. Any explicit `symptom.fingerprints` entry must match
   `^sha256:[0-9a-f]{64}$`.
6. `guard.repro`, when set, and each `guard.repros[].path`, must point to an
   existing file (corpus-level check; skipped in single-file validation).
   Duplicate `path` values within one record's `repros` are rejected.

## Versioning

`schema_version` is an integer. Breaking changes to this spec bump it and
add `schema/experience-record.v<N>.schema.json`; the parser refuses
versions it does not know. Records are never mass-migrated in place —
doctors migrate them one PR at a time (delta-updates only).
