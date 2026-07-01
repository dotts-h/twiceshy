---
id: 0075
title: intake-issues drainer — materialize the report_issue spool into docs/issues/ (uses #0066 capture)
status: closed
severity: medium
group: 0064
depends_on: []
forgejo: 448
links:
  adr:
  prs: [325]
  issues: []
  regression:
assets: []
---

## Summary
Split from #0066. #0066 shipped the `report_issue` MCP tool that captures
half-formed agent input to a quarantined spool (`spool.Issue` / `EnqueueIssue`),
plus a PR-ready docs/issues markdown fallback when no queue is configured. This
issue is the **drain half**: an `intake-issues` CLI subcommand (mirror
`intake-reports`) that reads the spool and materializes each `spool.Issue` into
`docs/issues/` with a freshly-allocated number + INDEX row.

## Approach
- `intake-issues -queue <dir>` mirrors `runIntakeReports`: `spool.List` →
  `spool.ReadIssue` → materialize → `spool.Remove`.
- **Reuse `scripts/new-issue.sh`** for numbering + INDEX append + template so the
  Go path and the script path don't drift on issue-number allocation (exp-0743's
  stale-id lesson applies to issue numbers too). Fill the created file's Summary
  with the description + category + author + related_record_id.
- **Dedup** at intake: skip a spooled issue whose normalized title already exists
  in `docs/issues/INDEX.md` (offline; not the hot path).
- Issues land triage-flagged (severity medium, agent-submitted note); a human or
  the triage doctor promotes them — never auto-actioned.

## Acceptance
- [x] `intake-issues` drains the spool into `docs/issues/NNNN-*.md` + INDEX rows.
- [x] Numbering reuses `new-issue.sh` (no second allocator); near-duplicate titles skipped.
- [x] Mirror to Forgejo via the existing `scripts/sync-forgejo.sh` — the materialized
      files are standard `docs/issues/` entries (with the empty `forgejo:` field
      `sync-forgejo.sh` fills), so the existing mirror path covers them unchanged.

## Status

Shipped: `twiceshy intake-issues -queue <dir> [-repo <dir>]`
(`cmd/twiceshy/intake_issues.go`) mirrors `intake-reports` — `spool.List` →
`spool.ReadIssue` → materialize → `spool.Remove`. Numbering, the INDEX append and the
file template come from the canonical `scripts/new-issue.sh` (shelled out, no second
allocator — exp-0743's stale-id lesson); the drainer fills only the body (description
in `## Summary`, category/author/related/security-flags in `## Notes`) and re-screens
the content. A spooled issue whose normalized title (case/space/punctuation-insensitive)
already exists in `docs/issues/INDEX.md` is skipped; malformed entries are logged and
removed so they cannot wedge a scheduled drain; an allocation/write failure aborts so
the entry is retried. Tested against the real `new-issue.sh` in a throwaway git repo
(`cmd/twiceshy/intake_issues_test.go`).

## Notes
Depends on #0066 (the tool + spool, merged). Mirrors `intake-reports`
(ADR-0013 §E1). Child of epic #0064.
