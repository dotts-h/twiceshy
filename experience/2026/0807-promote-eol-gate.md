---
schema_version: 1
id: exp-0807
kind: trap
status: quarantined
title: Scoping a guard to a status needs the matching exclusion on the path that produces it
symptom:
    summary: 'The D2 staleness guard was scoped to validated records (so the importer could ingest EOL-runtime advisories as quarantined drafts), but the promote side kept no mirror: the ADR-0016 advisory panel promoted those same EOL advisories to validated, where the guard immediately flagged them — reding TestStaleness_RealCorpusNotFalseFlagged and stalling ~36 validate PRs.'
    error_signatures:
        - committed corpus false-flagged as stale
        - TestStaleness_RealCorpusNotFalseFlagged
applies_to:
    - ecosystem: Go
      package: endoflife.date
resolution:
    root_cause: 'Scoping a guard to a status fixes the check but not the producer. #302 scoped D2 to validated records so quarantined EOL-runtime advisory drafts stopped tripping it on the import side. Nothing, though, stopped the advisory panel from PROMOTING such a draft — and the instant it became validated the (correct) guard flagged it. A one-sided scoping does not remove the failure; it relocates it to wherever the guarded status is next created.'
    fix: 'Expose the guard''s own predicate status-independently (doctor.Staleness.WouldFlag — the two staleness signals with no validated-status gate) and consult it on the producing path BEFORE the expensive work: promote.promoteAdvisory holds a born-stale advisory (EOL runtime, or valid.until already past) quarantined, before the panel. The producer and the guard now agree on what "stale" means. Fails open on an endoflife.date outage (no data ⇒ no flag), with the deterministic guard test as the backstop. Recorded as ADR-0016 §7.'
guard:
    repro: null
    guarding_test: 'internal/promote/promote_test.go::TestPromote_AdvisoryEOLRuntime_HeldNotPromoted (an advisory the gate flags is held, quarantined, without the panel being consulted) + internal/doctor/staleness_test.go::TestStaleness_WouldFlag_StatusIndependent (WouldFlag flags a quarantined EOL draft that Run, gated on validated, skips).'
provenance:
    source:
        author: claude
        session: twiceshy-0071-2026-06-22
        pr: null
    recorded_at: "2026-06-22"
    validated_at: null
    valid:
        from: "2026-06-22"
        until: null
    superseded_by: null
---

## What happened
#302 scoped the D2 staleness guard to **validated** records so the live importer
could ingest EOL-runtime advisories (e.g. a Python 3.8 vuln) as quarantined drafts —
a draft is not "drift." But the ADR-0016 advisory panel then promoted those same
advisories to `validated`, where the (now validated-scoped) guard correctly flagged
them. `TestStaleness_RealCorpusNotFalseFlagged` went red and ~36 mergeable
`validate/*` PRs piled up, none landing (2026-06-22).

## Why
The import side and the promote side are two paths that both *produce* corpus state,
and the fix had only touched one. Scoping the guard to validated stopped the import
draft from tripping it, but promotion is exactly the act of creating a validated
record — so a born-stale advisory promoted by the panel was a validated EOL record by
construction, and the guard did its job.

## The fix
`doctor.Staleness.WouldFlag` runs the same two signals as the guard (past
`valid.until`; a Fixed version on an EOL endoflife.date cycle) **without** the
validated-status gate. `promote.promoteAdvisory` consults it before the panel and
holds a born-stale advisory quarantined. The gate is wired in the `validate` command
against the real endoflife.date source (`TWICESHY_EOL_URL` overrides for
test/offline) and **fails open** — a source outage yields no flag, so promotion
proceeds and the deterministic guard test remains the backstop.

## Generalization / dead-end
The lesson generalizes past this bug: **when you scope a guard to a status, add the
matching exclusion on the path that produces that status.** Two dead-ends rejected:
loosening the guard test to tolerate validated EOL records (wrong — they *are* stale;
the guard is right, the producer was wrong); and putting the EOL check in the pure
`EligibleAdvisory` predicate (it is shared with the CLI dry-run preview and must stay
pure/cheap — no ctx, no network — so the lookup belongs in the ctx-having `Promote`
path). Direct companion to the import-side sibling exp-0097 (the same staleness/EOL
seam, the other direction) and #302.
