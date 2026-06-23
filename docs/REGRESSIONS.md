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
| fp-vtab | `fingerprint.Normalize` was not idempotent for a signature led by Unicode whitespace the regex `\s` class does not match (vertical tab, NEL, no-break space) — index-time and query-time fingerprints could diverge | the leading char survived pass 1 but the final Unicode-aware `strings.TrimSpace` stripped it on pass 2, re-anchoring a `/path` at `^` | `internal/fingerprint` · `TestNormalizeIdempotentOnLeadingUnicodeWhitespace`, `FuzzNormalizeIdempotent` (PR #360) |
| pii-email | the `pii:email` screen rule flagged npm/JS `name@version` (e.g. `react-native-fs@2.20.0`) as an email, marking JS/RN records un-promotable — blocking the corpus's first frontend entries | the email regex's domain side allowed a numeric "TLD", so `local@2.20.0` read like `local@domain.tld` | `internal/screen` · `TestScan_DoesNotFlagPackageVersionAsEmail` |

## Dead-ends (tried and rejected)

| what we tried | why it can't work |
|---------------|-------------------|
| escaping FTS5 special characters with backslashes | FTS5 has no backslash escaping; only wrapping each token in double quotes (with embedded `"` doubled) makes it a literal term (exp-0001) |
| `omitempty` on the *required* objects (`provenance.source`, `provenance.valid`) to tidy output | they are schema-`required`; omitting them produces schema-invalid frontmatter. `omitempty` belongs only on optional fields |
| treating the SQLite index as the source of truth (migrating it in place) | the index is derived state — the markdown corpus is authoritative; wipe-and-`Rebuild` is the only "migration" (ADR-0001) |
| putting literal secret-shaped fixtures (`ghp_…`, JWT, an `api_key="<random>"`) in the secret-detector's own tests (`internal/screen`) | the repo secret-scanner (`gitleaks detect`) scans the **whole commit range**, not just the working tree, and correctly flags them — failing CI and re-failing even after a *follow-up* fix commit (the secret persists in branch history; you must rewrite/squash to one clean commit). Fix: build the fixtures at run time — pattern shapes via `strings.Repeat`/split literals, the high-entropy one via a hash (`hex(sha256(seed))`) — so no literal token exists in any commit; the detector still sees the full shape. Keeps gitleaks fully active (no `.gitleaks.toml` allowlist weakening) |
| ranking two judge prompts from a single `judge-eval` run, or sharpening the judge with `think=true` | the judge model (gpt-oss:20b) is **non-deterministic at temperature 0** on boundary cases — the AGPL license record false-approved ~1 in 7 samples under the shipped prompt, so a repeat=1 false-approve count flipped 0↔1 between runs. Rank on **repeat=N majority** (`twiceshy judge-eval -repeat 5`); `think=true` made it *worse* (added a false-approve, slower). See exp-0046; gold set + scorer in `internal/judgeeval` |
