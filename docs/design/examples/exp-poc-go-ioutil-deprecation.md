<!--
  PROOF-OF-CONCEPT — not blessed corpus.
  Uses the PROPOSED schema (provenance.source_license / source_url, see
  docs/design/corpus-bootstrap.md §4). It lives under docs/design/examples/ on
  purpose: it is NOT under experience/, so it is documentation, not indexed
  corpus, and does not run through the validator. When the importer lands and
  the schema fields are accepted, a record like this moves to
  experience/2026/NNNN-...md with a real id and (after D3) a validated status.

  Source tier: maintainer deprecation + tool-assisted rewrite (codemod-class).
  This is the VALIDATED-CAPABLE, HOT-PATH-ELIGIBLE shape: it carries a real
  fingerprintable lint signature and an executable fail-to-pass guard.
  License: the *fact* of the deprecation is from the Go 1.16 release notes
  (facts — not copyrightable); the lint id is from staticcheck (MIT).
-->
---
schema_version: 1
id: exp-poc-0001
kind: fix
status: quarantined
title: "Go 1.16 deprecated io/ioutil — staticcheck SA1019 fires; move to os/io equivalents"

symptom:
  summary: >
    After upgrading to Go 1.16+, code calling `ioutil.ReadFile`,
    `ioutil.WriteFile`, `ioutil.ReadAll`, `ioutil.ReadDir`, `ioutil.TempFile`,
    `ioutil.TempDir`, or `ioutil.NopCloser` still compiles and runs, but
    `staticcheck`/`gopls` flag every call as deprecated (SA1019). The package
    keeps working for now, so the warning is easy to ignore until a linter in
    CI starts failing the build.
  error_signatures:
    - "SA1019: ioutil.ReadFile has been deprecated since Go 1.16: As of Go 1.16, this function simply calls os.ReadFile."
    - "SA1019: ioutil.WriteFile has been deprecated since Go 1.16"
    - "SA1019: ioutil.ReadAll has been deprecated since Go 1.16: As of Go 1.16, this function simply calls io.ReadAll."

applies_to:
  - ecosystem: "Go"
    package: "io/ioutil"
    versions: { introduced: "1.16", fixed: null }
    runtime: { go: ">=1.16" }

resolution:
  root_cause: >
    Go 1.16 (Feb 2021) redefined io/ioutil as a thin compatibility shim and
    marked it deprecated: its functions were re-homed to `os` and `io`. The
    code is not wrong — it is stale — so the only signal is a deprecation
    diagnostic, which silent autopilot edits tend to suppress rather than fix.
  fix: >
    Apply the mechanical 1:1 rewrite, then drop the now-unused import:
    ioutil.ReadFile→os.ReadFile, ioutil.WriteFile→os.WriteFile,
    ioutil.ReadAll→io.ReadAll, ioutil.ReadDir→os.ReadDir (note: returns
    []os.DirEntry, not []os.FileInfo — the one non-trivial caller change),
    ioutil.TempFile→os.CreateTemp, ioutil.TempDir→os.MkdirTemp,
    ioutil.NopCloser→io.NopCloser, ioutil.Discard→io.Discard.
  dead_ends:
    - tried: "blanket find/replace ioutil.ReadDir → os.ReadDir"
      why_it_failed: >
        os.ReadDir returns []os.DirEntry, while ioutil.ReadDir returned
        []os.FileInfo sorted by name; callers that read FileInfo fields (Size,
        ModTime) won't compile until they call entry.Info(). The rest of the
        rewrites are pure 1:1.

guard:
  repro: "docs/design/examples/repro-go-ioutil-deprecation.sh"
  guarding_test: null

provenance:
  source: { author: "twiceshy-importer", session: null, pr: null }
  source_license: "none (facts only)"
  source_url: "https://go.dev/doc/go1.16#ioutil"
  recorded_at: 2026-06-13
  validated_at: null
  valid: { from: 2021-02-16, until: null }
  superseded_by: null
  usage: { retrieved: 0, confirmed_helpful: 0, last_hit: null }
---

## The trap

`io/ioutil` still works in modern Go, so the deprecation is invisible at
runtime — until a project turns on `staticcheck` (or upgrades `gopls`) and CI
starts emitting `SA1019` on every `ioutil.*` call. An agent on autopilot tends
to silence the diagnostic (a `//nolint`, or worse, reverting the linter) rather
than perform the rename, because nothing is actually broken.

## Why it happens

Go 1.16 re-homed the `io/ioutil` helpers into `os` and `io` and left the old
package as a deprecated shim. This is a *staleness* signal, not a failure — the
exact case twiceshy's bi-temporal `applies_to` exists to capture: the record is
true **from** Go 1.16 onward (`versions.introduced: "1.16"`), with no `fixed`
bound because the package is deprecated, not removed.

## The escape

Apply the 1:1 rewrites listed in `resolution.fix`. The only one that is not a
pure rename is `ioutil.ReadDir → os.ReadDir`, which changes the return type
from `[]os.FileInfo` to `[]os.DirEntry` (call `entry.Info()` where the old
`FileInfo` fields were used). `staticcheck` clean confirms the fix.

## Why this record can become `validated`

Unlike a prose tip, this trap ships an **executable fail-to-pass guard**
(`repro-go-ioutil-deprecation.sh`): it writes a tiny program using
`ioutil.ReadFile`, runs `staticcheck` and asserts SA1019 fires (the trap is
real), applies the rewrite, and re-runs `staticcheck` asserting it is now clean
(the fix works). Once Doctor 3 (Phase 4) can run this in a sandbox, the record
is promotable to `validated`. The lint message is also a stable
**fingerprint**, so this record is eligible for the embedding-free hot path.
