---
id: 0061
title: 'Importer transcription-fidelity bugs: mis-scoped ecosystem, malformed/mis-cased path, fixed-null fix-text contradiction, source_url/advisory-id mismatch, Go major-version-suffix errors'
status: closed
severity: medium
group: 0015
depends_on: []
forgejo: 188
links:
  adr: ADR-0016
  prs: [272]
  issues: []
  regression: internal/record/advisory_defects_test.go
assets: []
---

## Summary

The OSV/GHSA live importer emits advisory records with **five** classes of
transcription defect that make the record internally inconsistent or mis-scoped.
A full Sonnet audit of all 85 quarantined advisory records (2026-06-20,
`runs/sonnet-advisory-audit.json`, PR #193) rejected **19** of them across these
classes (the other 66 were approved + promoted). The ADR-0016 panel correctly
*holds* such records, but they pollute the quarantine queue and would be poison if
a lenient judge approved them — the worst (Defect 4) actively mis-directs an agent
to an unrelated advisory.

## Repro

Inspect the cited records under `experience/2026/`. Each is produced by
`author: twiceshy-importer` from a public OSV/GHSA feed.

### Defect 1 — ecosystem mislabel (non-Go package labeled `ecosystem: Go`)
- **exp-0010** — `package: github.com/strukturag/libheif`, `ecosystem: Go`.
  `libheif` is a **C/C++** library; there is no Go module at that path.
- Expected: ecosystem reflects the package's actual ecosystem (or the record is
  not imported as a Go advisory).
- Actual: a C/C++ library is presented as a Go dependency vulnerability — would
  flag safe Go code as affected.

### Defect 2 — malformed package path (`https://` URL prefix)
- **exp-0022** — `package: https://github.com/dadrus/heimdall`.
- Expected: a clean Go module path, `github.com/dadrus/heimdall`.
- Actual: the `package` field carries the source URL verbatim, so the identifier
  is not a usable module path.

### Defect 3 — `fixed: null` + contradictory boilerplate fix-text
- **exp-0012, 0015, 0034, 0053, 0059, 0061, 0064, 0073, 0077, 0083** (10 of the
  19 — the single largest class) — `versions.fixed: null` (no fix published) yet
  `resolution.fix` is the template
  *"Upgrade affected packages past the fixed version…"*, which references a fixed
  version that does not exist.
- Expected: when `fixed` is null, the fix-text says no fix is published yet
  (OSV convention — a null `fixed` is valid), not "upgrade past the fixed version".
- Actual: self-contradictory record (claims to be unfixed and fixed at once).

### Defect 4 — `source_url`/advisory-id mismatch (the link leads to a DIFFERENT vuln)
- **exp-0013, exp-0020, exp-0042, exp-0066** — the record's GHSA id (title /
  `error_signatures`) does not match the advisory the `source_url` and fix link
  point to. E.g. exp-0066 is recorded as `GHSA-9wmc-rg4h-28wv` (kuma) but links to
  `GHSA-jhv4-f7mr-xx76` (an Envoy advisory); exp-0042 records
  `GHSA-77vh-xpmg-72qh` (image-spec) but cites `GHSA-mc8v-mgrf-8f4m`
  (distribution-spec).
- Expected: `source_url` cites the SAME advisory the record transcribes.
- Actual: an agent following the fix link is sent to an UNRELATED vulnerability —
  the most dangerous class (it mis-directs remediation, not just fails to match).

### Defect 5 — Go major-version module-suffix / case errors
- **exp-0032** — fabricated `github.com/prometheus/prometheus/v2` (Prometheus
  publishes no `/v2` module). **exp-0054** — the OPPOSITE: `github.com/traefik/traefik`
  OMITS the `/v2`,`/v3` suffixes Traefik actually requires. **exp-0075** — uppercase
  `github.com/lin-snow/Ech0` instead of the canonical lowercase module path.
- Expected: the path matches the module's real published major-version form and
  canonical (lowercase) case.
- Actual: a fabricated, missing, or mis-cased suffix never matches the real module
  in a Go dependency scan.

## Evidence

A full Sonnet (claude-sonnet-4-6) audit of all **85** quarantined advisory records
(2026-06-20, `runs/sonnet-advisory-audit.json`, PR #193) applied the ADR-0016
4-check rubric (meaning/scope/license/poison) and **rejected 19** across the five
classes above; the other **66 were approved + promoted**. This supersedes the
earlier 7-record Sonnet/Haiku A/B. One reclassification from that A/B: **exp-0032
is a genuine bug** (fabricated `/v2`), not a Sonnet over-rejection; only **exp-0009**
(OliveTin's calendar-versioned `3000.10.2`) remains a correct approve.

## Progress

- [x] **Defect 3 — `fixed: null` + contradictory fix-text** (the largest class, 10
      records). Fixed: `osvLiveFixText` (`internal/ingest/osvlive.go`) renders
      "upgrade past the fixed version" only when an affected range carries a fixed
      version, else "no fix is published yet". Guard:
      `internal/ingest/osvlive_test.go::TestOSVLiveSource_NoFixedVersionFixText`.
      Dogfooded as exp-0745. Pairs with #0062 (the judge side). *(This PR.)*
- [x] **Defect 1 — ecosystem mislabel** (non-Go package labeled `ecosystem: Go`).
      The deterministic consistency gate now carries the independently audited
      advisory+ecosystem+package fact for the libheif case and quarantines/blocks it
      as `consistency:ecosystem-package-mismatch`. It deliberately does not infer
      ecosystem from module-path syntax.
- [x] **Defect 2 — malformed package path** (`https://` URL as the package). Fixed:
      `normalizePackageName` (`internal/ingest/osvlive.go`) strips a leading
      http(s):// from the OSV `affected.package.name` so the identifier is the clean
      module path, not a link, and the URL form never reaches summary/title/body.
      Guard: `osvlive_test.go::TestOSVLiveSource_PackagePathStripsURLScheme`. *(This PR.)*
- [x] **Defect 4 — `source_url`/advisory-id mismatch** (the most severe — the link
      leads to a different vuln). Fixed: `osvLiveGHSAURL` now cross-checks the GHSA id
      embedded in each reference URL against the record's own id + aliases
      (`ghsaIDPattern`); a URL citing a *different* advisory is rejected and the
      source_url falls back to the always-correct osv.dev page for this record's id.
      Guard: `osvlive_test.go::TestOSVLiveSource_SourceURLCrossCheck`. *(This PR.)*
- [x] **Defect 5 — Go major-version-suffix / case errors** (fabricated/missing
      `/vN`, uppercase module paths). The consistency gate now quarantines and
      promotion-blocks the three independently audited source facts (Prometheus,
      Traefik, Ech0). It does not lowercase or rewrite paths and does not generalize
      from `/vN` or uppercase syntax, avoiding false positives on legitimate modules.

All five audited classes now have deterministic regression coverage. Structural
classes are detected generally; semantic scope/canonical-module facts are matched
only to the exact independently audited advisory coordinates, so ambiguous source
data is never guessed or auto-corrected. Every emitted #0061 flag is promotion-
blocking, including alias-aware source-URL mismatches.

## Notes

**Root causes:**
- Defects 1, 2 & 5 — the importer copies the OSV `affected` package ref without
  validating/normalizing the ecosystem + module identifier (PURL parsing,
  ecosystem cross-check, Go major-version-suffix + canonical-case normalization).
- Defect 3 — the `resolution.fix` template assumes a fixed version exists; it must
  branch on `fixed == null` (the largest class, 10 records).
- Defect 4 (most severe) — the importer attaches a `source_url`/fix link that does
  not correspond to the GHSA id it recorded; the advisory→URL mapping has drifted
  (likely an aliasing/ordering bug when an OSV entry carries multiple ids/refs).
  Cross-check that the cited URL's advisory id equals the record's id.

**Scope / impact:** medium — data quality, currently *caught* by the ADR-0016
panel (held, not served), but it inflates the quarantine queue and depends on the
judge being strict enough to reject (a lenient judge approves them — see the A/B,
where Haiku approved exp-0010 and exp-0022).

**Related:** [[ADR-0016]] (advisory-class panel promotion — these are the
mis-transcriptions the panel's *meaning/scope/poison* checks are meant to catch);
part of epic 0015 (corpus growth as a live feed). A regression guard should
assert the importer normalizes ecosystem/package and handles `fixed: null`
fix-text once fixed.
