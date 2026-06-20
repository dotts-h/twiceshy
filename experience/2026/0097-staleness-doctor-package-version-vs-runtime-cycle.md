---
schema_version: 1
id: exp-0097
kind: trap
status: quarantined
title: Staleness/EOL doctor reads a third-party package's version as a language runtime cycle
symptom:
    summary: 'An EOL/staleness check that maps an OSV ecosystem (Go) to a language runtime and compares a record''s Fixed version against endoflife.date runtime cycles false-flags third-party modules: kyverno "fixed in v1.16.2" is read as Go 1.16 (EOL 2022-08-01) and proposed stale.'
    error_signatures:
        - go 1.16 reached end-of-life 2022-08-01
        - committed corpus false-flagged as stale
applies_to:
    - ecosystem: Go
      package: endoflife.date
resolution:
    root_cause: 'Signal 2 of the staleness doctor mapped ecosystem→runtime product (Go→go) and compared majorMinor(applies_to.Versions.Fixed) against endoflife.date cycles for EVERY package in that ecosystem. A third-party module''s own semver (kyverno v1.16.2 → "1.16") coincidentally shares major.minor with an EOL''d runtime cycle (Go 1.16), so the module record gets flagged stale. The premise "a package version that isn''t a runtime cycle simply finds no match" is false: package versions and runtime cycles share the same small integer space and collide.'
    fix: 'Treat Versions.Fixed as a runtime release cycle only when the package denotes the runtime/stdlib itself. A third-party Go module is a domain-qualified import path (a host in the first path segment: github.com/…, k8s.io/…); the runtime is the empty package or a bare non-domain token (stdlib import paths like io/ioutil, or "go"). Gate signal 2 on isRuntimePackage(pkg): true when pkg is empty or its first ''/''-segment contains no dot.'
guard:
    repro: null
    guarding_test: internal/doctor/staleness_test.go::TestStaleness_SkipsThirdPartyModuleCycleCollision — a record with ecosystem Go, package github.com/kyverno/kyverno, Fixed 1.16.2 under an EOL Go-1.16 source must NOT be flagged, while a stdlib record (package io/ioutil, Fixed 1.16.0) on the same cycle still IS flagged.
provenance:
    source:
        author: claude
        session: twiceshy-adoption-2026-06-20
        pr: null
    recorded_at: "2026-06-20"
    validated_at: null
    valid:
        from: "2026-06-20"
        until: null
    superseded_by: null
---

## What happened
The scheduled OSV importer added a kyverno advisory record (`fixed: 1.16.2`). CI went red on `TestStaleness_RealCorpusNotFalseFlagged`: the D2 staleness doctor proposed the record `stale` with issue `go 1.16 reached end-of-life 2022-08-01`. But `1.16.2` is the **kyverno module** version, not the Go toolchain version.

## Why
`staleByEOL` mapped the OSV *ecosystem* (`Go`) to the endoflife.date *product* (`go`, the language) and compared `majorMinor(Fixed)` to the language's EOL cycles — for any package in the ecosystem. OSV "Go ecosystem" means "a Go module," and a module's version has nothing to do with the language release cadence. Earlier OSV batches merged only because no fixed-version's major.minor happened to land on an EOL'd Go cycle (1.16 / 1.20); kyverno's did.

## The fix
Signal 2 now fires only when `isRuntimePackage(applies_to.package)` — empty, or a non-domain token (stdlib import paths, or the bare runtime). Domain-qualified module paths (host in the first segment) are skipped, because their versions are package versions, not runtime cycles.

## Generalization / dead-end
The "dotted first segment ⇒ third-party module" discriminator is exact for Go (every module path is domain-qualified; stdlib paths are not). It is weaker for npm/PyPI, where a third-party package name has no dot — there a version/cycle collision is still possible (e.g. a package at v18 vs Node 18). The importer currently only feeds Go, so the Go-exact rule is sufficient today; a per-ecosystem "is this the runtime?" predicate is the follow-up if other ecosystems are imported.
