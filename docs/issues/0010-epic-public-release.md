---
id: 0010
title: "Epic: Public release (Tier B) — multi-tenant isolation, trial, anti-abuse"
status: open
severity: low
group:
depends_on: [0009]
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
**Future — not the near-term single-tenant deploy.** What's required before
twiceshy is served to *other people* for pay. Depends on #0009 (Tier-A security)
and the core epic #0008. Grounded in SECURITY_ANALYSIS.md Tier B.

## Children (break down when this is picked up)
- **Tenant isolation** (P0, non-negotiable): a `tenant_id` on every record + every
  query filters by it; per-tenant storage; one tenant's records/queries can never
  reach another's context. (SECURITY_ANALYSIS.md Facet 3.)
- **Per-tenant auth:** scoped per-tenant tokens (and/or OIDC), not one static
  bearer; record vs read-only scopes.
- **Free-trial window:** N days/1 week of access before payment is required.
- **Anti-abuse:** prevent trivial multi-account bypass of the trial — e.g. verified
  identity / payment-instrument fingerprint / device or email-domain heuristics;
  decide the mechanism (research needed — balance friction vs abuse).
- **Billing + entitlement** plumbing; **SBOM + release signing** (P2 from #0014).

## Notes
Filed now so it's tracked and so Tier-A work (#0009, #0013) leaves the right
seams (e.g. don't hard-code single-tenant assumptions that block `tenant_id`
later). Do **not** build until the single-tenant deploy is done and validated.
