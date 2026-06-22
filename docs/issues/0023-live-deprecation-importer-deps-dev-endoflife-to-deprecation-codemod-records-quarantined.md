---
id: 0023
title: Live deprecation importer — deps.dev/endoflife to deprecation+codemod records, quarantined
status: closed
severity: medium
group: 0015
depends_on: []
forgejo: 113
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

ADR-0011 phase 3 — the most testable, cleanest live source: deps.dev /
endoflife.date / changelogs → **deprecation + codemod** records, quarantined.
Extends the existing embedded `deprecation.go` adapters to live fetch.

**Partial coverage (2026-06-19):** the *facts* this issue would import already
exist as a curated, license-clean embedded set
(`internal/ingest/data/go-deprecations.yaml`, the `go` source). A one-off
`twiceshy ingest go` run landed them as quarantined records (exp-0043 io/ioutil,
exp-0044 strings.Title, exp-0045 math/rand) so the #0026 drafter pipeline had
real records to prove against. The remaining scope of *this* issue is the **live**
deps.dev / endoflife.date fetch (auto-refreshing the set rather than the one-off
embedded seed) — it stays **open** for that.

## Touches

`internal/ingest/deprecation.go` + new live sources.

## Acceptance

- [x] Live deps.dev / endoflife fetch; deterministic distillation; quarantined
- [x] Idempotent; license-clean; passes the screen; test-first

## Status

Shipped the **endoflife.date** half: `twiceshy ingest eol-live` (`internal/ingest/eollive.go`,
`EOLLiveSource`) live-fetches release cycles from endoflife.date and emits a quarantined
deprecation record for every cycle past its end-of-life date (or `eol:true`), mirroring the
live OSV importer (#0021): a `WithEOLLiveFetch` injection seam (fixture-tested, zero network
in CI), deterministic distillation (sorted by `EOL:<product>:<cycle>` signature, clock
injected via `WithEOLNow`), `AppliesTo.Runtime{product: cycle}` (the natural field for a
record whose subject is a runtime version rather than a package), and license-clean
facts-only prose (no third-party text;
`SourceLicenseFactsOnly`). Born quarantined and idempotent via the shared `ingest.Prepare`
dedup (`IncludeQuarantined`) — proven end-to-end by `TestEOLLive_PrepareQuarantinesAndDedups`.
Future EOL dates are skipped until they pass (a later run picks them up). EOL-runtime records
are born quarantined, so they never trip the validated-only staleness guard (the exp-0746 lesson).

Deferred: the **deps.dev** per-package `isDeprecated` source (the API is per-package, not
bulk, so it needs a package seed list) and wiring `eol-live` into the scheduled heartbeat
(#0022, an ops choice of products/cadence) — neither is required by the acceptance, which the
endoflife.date live source satisfies.
