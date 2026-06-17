# REGRESSIONS.md — twiceshy fixed bugs & their guards

> **Every fixed bug gets a guard test and a row here.** A fix with no guard is a
> bug on a timer. Each row names the symptom, the root cause in one line, and the
> exact test that now fails if it comes back. Dead-ends ("we tried X, it can't
> work because Y") are recorded too — they save the next session from re-walking
> the same path.

## Fixed bugs

| # | symptom | root cause | guard test |
|---|---------|------------|------------|
| exp-0001 | FTS5 `MATCH` on raw user/agent text throws `fts5: syntax error` on punctuation, or silently changes query semantics (`"` opens a phrase, `-`/`OR` become operators) | the right-hand side of `MATCH` is a query language, not a search string; raw text was passed through | `internal/index` · `TestSearchQuoteEscapesFTS5Input` |
| exp-0001b | a NUL (or other control) byte in a query token → FTS5 `unterminated string` crash | control runes reached the MATCH term; the dangling open-quote was never closed | `internal/index` · `FuzzSearchNeverErrors` (+ `stripControl`) |
| exp-0002 | relevance floor demoted strong matches and kept weak ones — inverted | SQLite `bm25()` is *negative*, smaller-is-better; the floor assumed higher-is-better | `internal/index` · `TestSearchRelevanceFloorUsesBM25Convention` |
| exp-0003 | a real MCP SDK client could not complete the handshake | server spoke the deprecated HTTP+SSE transport instead of streamable HTTP | `internal/server` · `TestServerSpeaksStreamableHTTP` |
| marshal | `record.Marshal` emitted JSON-Schema-invalid frontmatter (`symptom: null`, `runtime: {}`, `applies_to: []`, empty-string scalars) — and `record_experience` hands that markdown to an agent as a ready-to-PR file | optional YAML fields lacked `omitempty`; nothing validated Marshal output against the normative schema | `internal/record` · `TestMarshal_CorpusOutputSatisfiesSchema`, `TestMarshal_MinimalDraftSatisfiesSchema` |
| round-trip | the Marshal round-trip guard was tautological — it re-marshaled both sides, so a *symmetric* field drop (e.g. `error_signatures`, the dedup key) was invisible | comparison went through Marshal on both sides instead of comparing the parsed structs | `internal/record` · `eqIgnoringRaw` (structural, nil/empty-tolerant) |

## Dead-ends (tried and rejected)

| what we tried | why it can't work |
|---------------|-------------------|
| escaping FTS5 special characters with backslashes | FTS5 has no backslash escaping; only wrapping each token in double quotes (with embedded `"` doubled) makes it a literal term (exp-0001) |
| `omitempty` on the *required* objects (`provenance.source`, `provenance.valid`) to tidy output | they are schema-`required`; omitting them produces schema-invalid frontmatter. `omitempty` belongs only on optional fields |
| treating the SQLite index as the source of truth (migrating it in place) | the index is derived state — the markdown corpus is authoritative; wipe-and-`Rebuild` is the only "migration" (ADR-0001) |
