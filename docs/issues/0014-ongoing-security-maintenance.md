---
id: 0014
title: Ongoing security maintenance — dep currency, OSV self-dogfood, PR checklist
status: open
severity: medium
group: 0009
depends_on: []
forgejo:
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
- [ ] **Dogfood OSV/GHSA self-monitoring:** a routine that matches twiceshy's own
      `go.mod` against the advisory data the importer ingests (#0007) and alerts on
      a hit (issue/notification). Uses the product on itself.
- [ ] **PR security checklist:** add to the PR template — new endpoint? auth/input
      validated? secrets/PII stored? file I/O / path risk? new deps checked? tenant
      implications (Tier B)?
- [ ] **SBOM + release signing (P2):** syft/cyclonedx SBOM per release; cosign
      (Sigstore) signing of binaries/images; verify in the deploy path.

## Acceptance
- [ ] Dep-update PRs open automatically and run the gates.
- [ ] A known-vulnerable dep in twiceshy's own go.mod is surfaced by the dogfood
      monitor (test with a synthetic advisory).
