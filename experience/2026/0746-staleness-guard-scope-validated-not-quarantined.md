---
schema_version: 1
id: exp-0746
kind: trap
status: validated
title: "A whole-corpus guard/doctor that judges quarantined drafts blocks legitimate ingestion — scope it to validated records"

symptom:
  summary: >
    A guard test that runs a corpus doctor over EVERY committed record (including
    quarantined drafts) and asserts zero findings will go red the moment an
    ingested draft legitimately trips the doctor — even though the draft is correct.
    Here the D2 staleness doctor flagged a quarantined imported advisory that
    targets an end-of-life runtime (an aiohttp GHSA on Python 3.8, EOL 2024-10-01)
    as "stale"; the guard test failed CI, every live-importer PR went red, and the
    auto-merge correctly refused them — so the corpus silently froze at 745 records
    for ~12h with no alert (the importer swallowed the merge result with `|| true`).
  error_signatures:
    - "committed corpus false-flagged as stale"

applies_to:
  - ecosystem: "Go"
    package: "github.com/dotts-h/twiceshy"

resolution:
  root_cause: >
    Two compounding mistakes. (1) The staleness doctor evaluated records of any
    status, but it only ever proposes a validated→stale demotion — a quarantined
    draft cannot become stale, and a draft "born" about EOL tech is not world
    *drift*, so judging it is meaningless. (2) The guard test asserted the property
    over the WHOLE corpus, coupling "the served corpus is clean" to "no draft ever
    trips the doctor" — so ingesting any draft the doctor would flag broke the gate.
  fix: >
    Scope the doctor to the served subset: it now evaluates ONLY `validated`
    records (quarantined/disputed/retired are skipped). The whole-corpus guard test
    then passes because draft imports are exempt, and the importer can ingest
    advisories for EOL runtimes (born quarantined) freely. Separately, never swallow
    an auto-merge result silently — alert on a left-open red PR so a stall can't hide.
  dead_ends:
    - tried: "Filtering EOL-runtime advisories out at ingest so the guard stays whole-corpus."
      why_it_failed: >
        Throws away real, valid security knowledge (a vuln on a dead runtime is
        still true) and contradicts the goal of importing everything; the guard, not
        the importer, was mis-scoped.

guard:
  guarding_test: "TestStaleness_EvaluatesOnlyValidated"

provenance:
  source:
    author: "claude"
    session: "get-next-0065-then-corpus"
    pr: null
  recorded_at: "2026-06-22"
  validated_at: "2026-06-22"
  valid:
    from: "2026-06-22"
    until: null
---

# A whole-corpus guard that judges quarantined drafts blocks ingestion

twiceshy's live OSV importer opens a PR per batch and auto-merges it on green. For
~12h every import PR was red and the corpus was frozen at 745 records — but the
imports themselves were fine (60 new records each).

The culprit was a **guard test**, `internal/doctor/staleness_test.go`'s
`TestStaleness_RealCorpusNotFalseFlagged`: it ran the D2 staleness doctor over the
**entire committed corpus** and asserted **zero** findings. That implicitly assumed
no committed record would ever trip the doctor. The assumption held until the
importer ingested an advisory for an **end-of-life runtime** (aiohttp on Python 3.8).
The staleness doctor — whose job is to propose `validated → stale` for records the
world has drifted past — dutifully flagged it. Guard red → CI red → the auto-merge
helper (rightly) refused to merge a red PR → ~15 import PRs piled up open, all also
**colliding on ids** (each allocated `exp-0746+` off the frozen `main`).

The fix is a one-line scope change with a clear rationale: **staleness evaluates
only `validated` records.** A quarantined draft cannot transition to stale, and a
draft *born* about EOL tech is not drift. With the doctor scoped to the served
subset, the whole-corpus guard passes (drafts are exempt) and the importer can pull
EOL-runtime advisories without tripping it.

**The reusable lesson:** when a guard test runs a "doctor"/linter over *all* stored
records and asserts a clean result, it silently couples *"the served set is healthy"*
to *"no draft ever trips the check."* Scope such guards to the subset the property is
actually about (here: `validated`). And when an automated pipeline swallows a merge
result (`… || true`), a hard failure produces **zero signal** — alert on a left-open
red PR so a freeze can't hide.
