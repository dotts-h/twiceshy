# ADR-0035: Feature-gated organization, workspace, and plan foundations

- **Status:** Proposed (2026-07-10)
- **Related:** ADR-0030 (hosted multi-tenant MCP), ADR-0034 (durable tenant
  registry), ADR-0021 (future per-tenant corpus stores).

## Context

Tenant tokens currently carry a label and two directly configured quotas. A
private-team product needs stable organization/workspace identity and a plan
entitlement vocabulary before billing or isolated private corpora can be added.
Changing alpha signup or operator authentication while building that foundation
would create unnecessary launch risk.

## Decision

Add organization, workspace, and token-entitlement tables to the durable tenant
registry. Plans are closed, validated identifiers (`community`, `pro`, `team`,
`enterprise`) whose entitlements derive the stored request quota policy. Legacy
token issuance remains the default path.

The new issuance, assignment, and reporting CLI surfaces are gated by
`TWICESHY_TEAM_PLANS`; the flag is disabled unless explicitly set to `1`,
`true`, or `yes`. The operator bearer token bypass remains unchanged. The
existing signup path continues issuing alpha tokens with its existing explicit
quotas and no organization/workspace assignment.

This phase makes no external calls and contains no payment-provider, checkout,
price, email, seat-enforcement, or private-corpus behavior. Workspace identity
is metadata until a later isolation ADR defines its authorization boundary.

## Consequences

- Existing databases migrate additively: new tables are created without
  rewriting or deleting tokens and usage counters.
- Corpus rebuilds must preserve the new tenant-registry tables.
- Plan quota defaults are implementation policy, not published pricing; a later
  commercial decision may revise them before the feature flag is enabled.
- A later private-corpus implementation must not treat the metadata alone as an
  isolation guarantee.
