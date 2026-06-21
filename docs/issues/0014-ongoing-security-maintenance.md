---
id: 0014
title: Ongoing security maintenance — dep currency, OSV self-dogfood, PR checklist
status: open
severity: medium
group: 0009
depends_on: []
forgejo: 104
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
SECURITY_ANALYSIS.md Facet 4 — a durable process so the app stays secure after
deploy. `govulncheck` + `gitleaks` already run in CI; add the rest.

## Scope
- [ ] **Automated dependency updates:** Renovate (or Dependabot) for `go.mod` →
      PRs that trigger CI (`govulncheck`/tests). Confirm Forgejo support / runner.
      *(Deferred — ops: a Renovate bot adds to an already-busy PR queue (30+ cron
      PRs); confirm desired + provision a runner/token before enabling.)*
- [x] **Dogfood OSV/GHSA self-monitoring:** `twiceshy self-audit` (`internal/selfaudit`)
      matches every module in `go.mod` (direct + indirect) against the ingested
      vulnerability advisories and exits non-zero on a hit so a timer/CI alerts.
      Recognizes GHSA/CVE/GO **and MAL-/RUSTSEC-** (broader than `IsAdvisoryClass`,
      so a malicious package in a dep is not missed).
- [x] **PR security checklist:** added as `.forgejo/PULL_REQUEST_TEMPLATE.md`.
- [ ] **SBOM + release signing (P2):** syft/cyclonedx SBOM per release; cosign
      (Sigstore) signing of binaries/images; verify in the deploy path. *(P2 — ops.)*

## Acceptance
- [ ] Dep-update PRs open automatically and run the gates. *(Renovate — deferred, above.)*
- [x] A known-vulnerable dep in twiceshy's own go.mod is surfaced by the dogfood
      monitor (test with a synthetic advisory). *(`internal/selfaudit` unit tests +
      a `cmd/twiceshy` CLI test with an on-disk synthetic GHSA advisory.)*

## Status

The **dogfood monitor** (the testable security core) and the **PR checklist** shipped.
Issue stays **open** for the two ops/P2 items — Renovate auto-updates (needs an ops
decision + a runner) and SBOM/signing (P2) — which need infra, not code.
