---
id: 0020
title: internal/repro revalidate doctor — version matrix + attestation, report-only (Go first)
status: closed
severity: high
group: 0015
depends_on: [0016, 0018, 0019]
forgejo: 110
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

The validation harness proper (ADR-0011 §3): `internal/repro.Revalidator`
implementing `doctor.Doctor` — runs each record's **test-set** in the broker
across a **version matrix**, proves fail→pass, **empirically derives `applies_to`
bounds**, and emits a `Finding` + a structured **attestation** (what ran, image
digests, versions, results). **Report-only** — a human flips `validated` /
`validated_at` in the PR (promotion never automatic). Go ecosystem first.

## Touches

`internal/repro` (Revalidator); CLI `twiceshy doctor revalidate`.

## Acceptance

- [x] Revalidator implements `Doctor`; report-only (no record mutation)
- [x] Runs positive + negative tests across the matrix; attestation emitted
- [x] Derives + sanity-checks `applies_to` version boundaries from results
- [x] Unit-tested with a stub broker; integration test on one real Go record
- [x] Depends on 0016 (schema), 0018 (broker), 0019 (screen)

## Resolution (done 2026-06-18)

`internal/repro.Revalidator` implements `doctor.Doctor` (report-only): for each
record carrying a repro test-set it runs every repro (positive + negative, legacy
`guard.repro` treated as positive) through the #0018 broker across a version
matrix, interprets the exit-code convention (0 = claim holds, 75 = skip, else =
world drifted), and emits a `doctor.Finding` (propose `validated` / `stale` /
inconclusive) plus a structured `Attestation` (image digests, per-repro exit
codes, the matrix labels it reproduced under). It never mutates a record — a
human flips `validated`/`validated_at` in the PR.

`applies_to` derivation: it records the matrix labels where the whole set held
(`reproduced_under`) and sanity-checks them against any declared `runtime.go`
bound, surfacing the empirical range for the reviewer (the human sets the final
bound). With more pinned toolchain images in the matrix this tightens to a real
version range.

CLI: `twiceshy doctor revalidate -corpus <dir> [-json] [-attest]` (needs
Docker/runsc; the socketless CI runner can't run it). Unit-tested with a stub
broker (holds→promote, broken→stale, skip→inconclusive, no-mutation,
path-traversal, multi-entry-all-must-hold, legacy repro); integration-tested
end-to-end on a real stdlib Go record under real runsc.

Dogfood trap found + recorded: `go test` inside the sandbox fails with `permission
denied` because it compiles the test binary into `$TMPDIR` (=`/tmp`), which the
broker mounts `noexec`; the fix is to point `TMPDIR` at the exec-able `/work`
volume. (recorded via the harness.)
