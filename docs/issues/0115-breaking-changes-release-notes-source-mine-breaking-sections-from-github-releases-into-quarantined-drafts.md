---
id: 0115
title: Breaking-changes release-notes source: mine BREAKING sections from GitHub releases into quarantined drafts
status: open
severity: high
group: 
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: [0022, 0073, 0106]
  regression:
assets: []
---

## Summary
**Reframed from this issue's title** to the researched scope: v1 mines
**Node.js changelogs** (MIT, Node-authored; `docs/WEB_SOURCES.md` row 14), not
generic GitHub releases — the GitHub-issues no-go documented in the same table
(row 16: "no-go for generic issue-prose ingestion", API access is not a
redistribution license) still stands, so this source parses Node's own
changelog files, never issue/PR prose. The `(SEMVER-MAJOR)` commit markers in
`doc/changelogs/CHANGELOG_V*.md` are machine-regular and are exactly the
post-cutoff breaking changes a pre-cutoff model gets wrong — the kind of
systematically-wrong-post-cutoff material wave 2 is meant to grow the corpus
with. New `ingest.Source` `"node-breaking"`, following the npm-deprecation
template (`internal/ingest/npmlive.go`): injected fetcher for testability,
30s HTTP timeout, 404-skip on an unknown/missing version, facts-only prose
built via `fmt.Sprintf` (never the maintainer's changelog text verbatim), a
synthetic error signature, and `SourceLicense`/`SourceURL` per ADR-0003 §4's
licensing rule. Registration follows the existing `importSource()` switch
(`cmd/twiceshy/main.go:530`). One draft per `SEMVER-MAJOR` entry (one lesson
per record), `kind: trap`, born quarantined (git-PR trust boundary, same as
every other importer) — the usefulness judge (#0110) and panel gate its
promotion like any other draft. Follow-up adapters per the `WEB_SOURCES.md`
table (Next.js row 13, Django row 10, Python "What's New" row 11) reuse the
same `Source` shape once this one lands.

## Repro
1. Run the new `node-breaking` importer against a tagged Node.js changelog
   file containing `SEMVER-MAJOR` entries.
Expected: one quarantined `trap` draft per `SEMVER-MAJOR` entry, facts-only
prose, `SourceLicense`/`SourceURL` set, deduped against the existing corpus.
Actual: no such source exists; breaking-change coverage in the corpus is
whatever has been hand-authored so far.

## Evidence
- `docs/WEB_SOURCES.md` row 14: Node.js changelogs/deprecations are
  "ingest-OK for Node-authored material — MIT" with `SEMVER-MAJOR` markers
  called out as machine-regular.
- `docs/WEB_SOURCES.md` row 16 and its "Explicit no-gos" section: generic
  GitHub issue/comment ingestion is a no-go (API accessibility is not a
  redistribution license) — this issue does not touch that surface.
- `internal/ingest/npmlive.go` is the adapter template: `WithNpmFetch`
  (injected fetcher), `npmLiveFetcher`'s 30s `http.Client{Timeout: ...}` and
  404→nil-skip, `npmDraft`'s `fmt.Sprintf`-built facts-only prose, and
  `SourceLicense: record.SourceLicenseFactsOnly` / `SourceURL`.
- `docs/adr/ADR-0003-corpus-bootstrap-source-scope.md` decision item 4 ("the
  licensing rule is normative and mechanically enforced") is the rule this
  source's facts-only prose and `SourceLicense`/`SourceURL` fields satisfy.
- `cmd/twiceshy/main.go:530` (`importSource`'s `switch`) is where every
  existing source is registered (`npm-deprecation` at line 546); the new
  source follows the same pattern.

## Acceptance
- Hermetic tests: fixture-driven changelog parsing, a malformed-entry-skip
  case, a dedup round-trip against an existing draft, and context-cancel
  propagation mid-fetch (mirroring `npmlive_test.go`'s coverage of the
  npm-deprecation source).
- A bounded live run against the real Node.js changelog produces schema-valid
  quarantined drafts (`record.Validate` clean).
- `docs/WEB_SOURCES.md` gains a "Recommended adapters" section entry for the
  Node.js changelog adapter, matching the existing Rust/Ruff/Clippy entries'
  format.

## Notes
The GitHub-issues no-go from `docs/WEB_SOURCES.md` stands unchanged — this
issue mines Node-authored changelog text only, never issue/PR/comment prose.
The issue's own title still names "GitHub releases"; the body above is the
researched reframe superseding it (kept per "keep frontmatter" — the title is
not edited here).
