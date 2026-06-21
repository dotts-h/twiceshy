---
schema_version: 1
id: exp-0743
kind: trap
status: quarantined
title: record_experience allocates a colliding id when the live index has drifted behind the committed corpus
symptom:
    summary: 'An id allocator that reads MAX(id)+1 from a derived SQLite index hands back an already-taken id whenever that index lags the source-of-truth files. twiceshy''s record_experience allocated exp-0016 for a novel draft though exp-0016 already existed on disk — and repeated it for a second draft — because the live server''s index was built at startup and the committed corpus had grown past it (no live sync). The collision is single-threaded: it needs no concurrency, only a stale index.'
    error_signatures:
        - record_experience returns an id that already exists on disk
        - NextID MAX(id)+1 over a stale index collides single-threaded
applies_to:
    - ecosystem: Go
      package: github.com/dotts-h/twiceshy
resolution:
    root_cause: 'Two compounding factors. (1) NextID computes MAX(id)+1 over the *index* (a derived SQLite table), not the corpus, so it trusts a view that can lag the filesystem. (2) A long-running server builds its index once at startup; imports then grow the on-disk corpus past it (the no-sync drift of #0060), so MAX falls far below the true filesystem maximum and the next id is already taken. Distinct from the concurrency hazard (TECH_DEBT M3, two simultaneous callers): this collides with a single caller.'
    fix: 'Allocate against the source of truth, not a derived view. ingest.NextID returns one past max(index max, on-disk corpus max); record.MaxID scans the experience/ tree by path only (never parsing a body, so a malformed record cannot break allocation). Because records hit disk before they are indexed, the disk max is always >= the index max, so this is correct even when the index is arbitrarily stale. All three index-based write paths — the importer, record_experience, report_outcome — route through it, and the server is handed its corpus root. Closes the stale-index hazard; the M3 concurrency race remains, acceptable only because the write path is propose-only (a colliding draft is caught in PR review).'
guard:
    repro: null
    guarding_test: internal/ingest/nextid_test.go::TestNextID/disk_ahead_of_a_stale_index_wins — builds an empty (maximally stale) index plus an on-disk record at exp-0016, then asserts the allocator returns exp-0017, not the stale index's exp-0001. internal/record/maxid_test.go::TestMaxID covers the disk scan (max across years, non-record files ignored, ids wider than four digits).
provenance:
    source:
        author: claude
        session: twiceshy-get-next-2026-06-21
        pr: null
    recorded_at: "2026-06-21"
    validated_at: null
    valid:
        from: "2026-06-21"
        until: null
    superseded_by: null
---

## What happened
On 2026-06-20 `record_experience` handed out **exp-0016** for a novel draft, but
`0016-…vault.md` already existed in the committed corpus. A second call repeated
exp-0016. The session worked around it by assigning the true next-free ids by hand
(exp-0097, exp-0098). Root cause: `Index.NextID` is a `MAX(CAST(substr(id,5) AS INT))+1`
read over the **index** — the live server's index was built at startup and the
on-disk corpus had grown past it, so `MAX` was far below the real maximum.

## Why this is distinct from the known concurrency hazard
`TECH_DEBT.md` M3 already notes `NextID` is non-atomic: two *concurrent* callers can
be handed the same id. That is a different failure mode. This one needs no
concurrency — a single caller collides because the index it reads is stale.

## The fix: allocate from the source of truth
`record.MaxID(root)` walks `experience/` and takes the highest `exp-NNNN` from the
**file paths** (no body parse, so a broken record can't wedge allocation).
`ingest.NextID(ctx, ix, corpusRoot)` returns one past `max(indexMax, diskMax)`. Since
a record is written to disk before it is indexed, `diskMax >= indexMax` always, so the
result is correct for an arbitrarily stale index. The importer, `record_experience`,
and `report_outcome` all use it; the server now carries its corpus root for the scan.

## Dead ends
- Trusting the index and "verifying the candidate id is unused" *in the index*: the
  collision is with a disk record the index never had, so an index-only check passes
  the bad id. The check has to consult the files.
- Coupling `index.Index` to the filesystem (giving it a corpus root): violates the
  SQLite-only retrieval seam. The disk scan belongs in `record`; the write-path
  composition belongs in `ingest`.

The `intake-reports` path already allocated from `maxRecordNum(LoadCorpus(...))` (the
disk), which is why it never hit this — the same source-of-truth principle, now applied
to the index-based write paths too. The concurrency hazard (M3) is unchanged.
