---
schema_version: 1
id: exp-0002
kind: trap
status: validated
title: "SQLite FTS5 bm25() is negative and lower-is-better — 'higher is better' floors and DESC sorts return garbage"

symptom:
  summary: >
    FTS5 ranking code that assumes conventional BM25 (bigger score = better
    match) misbehaves silently: `ORDER BY bm25(t) DESC` (or `ORDER BY rank
    DESC`) returns the *least* relevant rows first, and a relevance floor
    written as `score >= threshold` with a positive threshold rejects every
    row — search "works" in the demo and returns nothing or worst-first in
    production. No error is ever raised.
  error_signatures: []

applies_to:
  - ecosystem: "sqlite"
    package: "fts5"
    runtime: { sqlite: ">=3.9.0" }
  - ecosystem: "Go"
    package: "modernc.org/sqlite"

resolution:
  root_cause: >
    SQLite defines bm25() to return the negated BM25 score so that better
    matches have *smaller* values and `ORDER BY bm25(t)` (plain ASC) ranks
    naturally; the `rank` auxiliary column follows the same smaller-is-better
    convention. Every textbook, Lucene and Elasticsearch present BM25 as
    higher-is-better, so the sign convention is exactly the kind of fact an
    agent's training data steamrolls.
  fix: >
    Sort ascending (`ORDER BY rank` / `ORDER BY bm25(t)`), and express the
    relevance floor in the same convention: keep a row only when
    `bm25(t) <= -floor` for a positive floor. Convert to a positive
    "relevance" number at one single boundary (negate when mapping rows out)
    and document the convention there.
  dead_ends:
    - tried: "ORDER BY rank DESC, by analogy with every other search engine"
      why_it_failed: >
        rank is smaller-is-better too; DESC is precisely backwards and no
        test that only checks "results came back" catches it.
    - tried: "taking ABS(bm25(t)) to make scores 'normal'"
      why_it_failed: >
        Obscures the convention instead of handling it: any later code that
        compares raw bm25() output against the absolute-value floor is wrong
        again, and the boundary where the sign flips becomes untraceable.

guard:
  repro: null
  guarding_test: "TestSearchRelevanceFloorUsesBM25Convention"

provenance:
  source: { author: "horia", session: null, pr: null }
  recorded_at: 2026-06-12
  validated_at: 2026-06-12
  valid: { from: 2026-06-12, until: null }
  superseded_by: null
  usage: { retrieved: 0, confirmed_helpful: 0, last_hit: null }
---

## The trap

This is a *silent* trap — there is no error signature to fingerprint, which
is why this record exists: lexical search over the summary is the only
retrieval surface. You write a relevance floor (`score >= 0.5`), your
integration test asserts "query returns the seeded row", everything is
green. In reality `bm25()` returned `-1.37`, your floor filtered it, and
the only reason the test passed is that it didn't apply the floor — or it
sorted DESC and the assertion only checked membership, not order.

## Why it happens

SQLite wants `ORDER BY rank` with no modifier to "just work", so it negates
BM25: better match → more negative. This is documented, correct, and
opposite to the convention in every other system an LLM has read about.
Agents on autopilot pattern-match "BM25 score threshold" to
higher-is-better and produce comparisons with the sign flipped.

## The escape

Adopt SQLite's convention everywhere inside the query layer (`ORDER BY
rank` ASC, floor as `bm25(t) <= -floor`), and flip the sign exactly once at
the exit boundary if callers want a positive relevance number. twiceshy's
relevance floor (ADR-0001 §3 — below the floor, *nothing* is injected) is
implemented this way; the guarding test pins both the ordering and the
floor semantics with a relevant and an irrelevant document.

## Scope

Any SQLite FTS5 user in any language. Not applicable to Lucene-family
engines (Elasticsearch, OpenSearch, Tantivy) — there BM25 really is
higher-is-better, which is what makes this a trap.
