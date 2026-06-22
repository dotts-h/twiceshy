---
schema_version: 1
id: exp-0001
kind: trap
status: validated
title: "SQLite FTS5 MATCH treats user input as query syntax — dots, dashes and quotes error out or change semantics"

symptom:
  summary: >
    Passing a raw user/agent query string to `... WHERE t MATCH ?` throws
    `fts5: syntax error` as soon as the string contains punctuation that is
    legal in identifiers (`modernc.org/sqlite`, `utf-8`, `node.js`) — or,
    worse, silently changes meaning: a stray `"` opens a phrase, `-` and
    uppercase `NOT`/`OR`/`AND` become operators, `^` anchors, `*` expands.
  error_signatures:
    - 'fts5: syntax error near "."'
    - 'fts5: syntax error near "-"'
    - 'fts5: syntax error near "/"'
    - 'unterminated string'

applies_to:
  - ecosystem: "Go"
    package: "modernc.org/sqlite"
  - ecosystem: "Go"
    package: "github.com/mattn/go-sqlite3"
  - ecosystem: "sqlite"
    package: "fts5"
    runtime: { sqlite: ">=3.9.0" }

resolution:
  root_cause: >
    The right-hand side of MATCH is not a string to search for — it is a
    query language (AND/OR/NOT/NEAR, column filters, `"phrases"`, `^`, `*`,
    `-`). FTS5 bareword tokens may contain only alphanumerics, `_` and
    codepoints ≥ 0x80, so any plain-looking identifier with `.`, `/` or `-`
    is a syntax error, and there is no backslash escaping. Parameter binding
    (`MATCH ?`) does not help: it prevents SQL injection, not FTS5 query
    injection.
  fix: >
    Never hand raw text to MATCH. Split the input into tokens, drop empties,
    double each embedded `"` and wrap every token in double quotes
    (`"modernc.org/sqlite"` `"utf-8"`), then join with a space (implicit
    AND). Quoted strings are always plain terms, never operators.
  dead_ends:
    - tried: "escaping the special characters with backslashes"
      why_it_failed: >
        FTS5 query syntax has no backslash escape; the backslash is just
        another disallowed bareword character and errors out itself.
    - tried: "wrapping the whole user string in one pair of double quotes"
      why_it_failed: >
        That turns the entire input into a single phrase query: multi-word
        input then only matches the exact adjacent word sequence, silently
        destroying recall — and an embedded `"` in the input still breaks
        the quoting.
    - tried: "quoting each token but leaving its raw bytes untouched"
      why_it_failed: >
        A token can still carry control bytes the tokenizer can never index.
        A NUL byte (`\x00`) terminates the FTS5 query string mid-token, so the
        dangling open quote reports `unterminated string`. Strip control runes
        from each token before quoting it; whitespace controls are already
        gone via the token split. Found by fuzzing (FuzzSearchNeverErrors).

guard:
  repro: "experience/repro/0001-fts5-raw-match.sh"
  guarding_test: "TestSearchQuoteEscapesFTS5Input"

provenance:
  source: { author: "horia", session: null, pr: null }
  recorded_at: 2026-06-12
  validated_at: 2026-06-12
  valid: { from: 2026-06-12, until: null }
  superseded_by: null
  usage: { retrieved: 0, confirmed_helpful: 0, last_hit: null }
---

## The trap

You build search on SQLite FTS5, dutifully use a bound parameter
(`WHERE t MATCH ?`), test it with `hello world`, and ship. The first real
query containing a package name, version string, or error message —
`modernc.org/sqlite`, `utf-8`, `exit code 1` — either throws
`fts5: syntax error near "."` or quietly does something else than searching
for those words. On autopilot, an agent "fixes" the syntax error by
stripping punctuation, which destroys exactly the identifier-heavy tokens
that make error-message search work.

## Why it happens

The MATCH right-hand side is a full query language, not a needle. Barewords
admit only alphanumerics, `_`, and codepoints ≥ `0x80`; everything else is
operator territory: `"` opens a phrase, `-` is a column-exclusion prefix,
`^` anchors to the start, `*` is a prefix expansion, and bare `AND`, `OR`,
`NOT`, `NEAR` are operators. Parameter binding protects against SQL
injection only — the *FTS5 parser* still parses the bound string.

## The escape

Treat user text as data by construction: tokenize on whitespace, double any
embedded `"` inside each token, wrap each token in double quotes, join with
spaces (implicit AND). Inside double quotes there are no operators. If
OR-semantics are wanted, join the quoted tokens with `OR` explicitly —
that's then *your* operator, not the user's.

## Scope

Applies to every SQLite FTS5 binding in any language — the trap lives in
SQLite, not in the driver. (FTS3/4 has the same shape with a different
grammar.) twiceshy's own index hot path does exactly this quoting; the
guarding test walks the punctuation corpus through the live index.
