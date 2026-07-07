# ADR-0034: The tenant registry is not derived state — name it, pin it, split it later

- **Status:** Accepted (2026-07-07; horia approved the 2026-07-07 architecture
  review's proposal; phase 1 ships now, the physical split is deferred).
- **Related:** ADR-0001 §3 (the index as derived, always-rebuildable state — the
  contract this amends), ADR-0030 (the token layer that changed the facts),
  ADR-0032 (`contribution_usage`, the fourth durable table), #0125 (tokens),
  #0131 finding 2 (`token_usage`), #0126 (`tenant_usage`), issue 0137.

## Context

ADR-0001 defines the SQLite index as **derived**: the markdown corpus is the
source of truth, and the db file is always rebuildable from it. CONTRACTS.md
carved out one exception (usage counters). The alpha token layer quietly added
four more kinds of state to the same file: `tokens`, `token_usage`,
`tenant_usage`, and `contribution_usage` (ADR-0032).

`tokens` is different in kind from every prior exception: it holds tenant
**credentials**, stored hash-only — irrecoverable by design. The "delete the
db and re-run `twiceshy index`" mental model the docs teach would, applied to
a tenant-serving deployment, revoke every issued token permanently; the only
recovery is re-signup of the whole cohort. Today nothing pins the safety that
exists: `Rebuild`'s DELETE set happens not to touch the tenant tables, but no
test asserts it, so a refactor of `Rebuild` could silently start wiping
credentials and CI would stay green.

## Decision

Recognize and protect the tenant registry now; physically separate it later.

**Phase 1 (this ADR's shipped scope):**

1. **A guarding test** pins the invariant: `Rebuild` preserves `tokens`,
   `token_usage`, `tenant_usage`, and `contribution_usage` — issued tokens
   still authenticate, counters keep their values and keep counting, after a
   full corpus rebuild.
2. **The contract stops lying.** CONTRACTS.md and CODEBASE_MAP.md describe the
   db file as *derived index + durable tenant registry*, and name the
   never-delete consequence for tenant-serving deployments.
3. **The runbook says it out loud.** DEPLOY-public-alpha.md's backup section
   is updated to the as-built state (crash-consistent `sqlite3 .backup` on the
   VPS, nightly brain-side pull with integrity check and retention) and warns
   that the volume holds the tenant registry: deleting/recreating it is a
   credential-revocation event, not a cache rebuild.

**Phase 2 (deferred, with an explicit trigger):** move the tenant registry to
its own SQLite file behind its own store type, restoring the index file's
honest disposability. Trigger: before any multi-instance deployment, or the
first time an index-file migration/rebuild bug threatens the registry —
whichever comes first. Deferred because the alpha runs one instance, the
backup now covers the risk's operational half, and a live-service migration
spends risk that phase 1 removes more cheaply.

## Options considered

- **Split the file now — rejected for the alpha:** cleanest end-state, but a
  data migration on a live service to remove a risk that a test + docs + the
  existing backup already bound; revisit at the phase-2 trigger.
- **Docs only, no test — rejected:** the invariant would remain accidental;
  `Rebuild` refactors are routine and CI must catch the regression, not an
  operator's incident review.
- **Recognize + pin now, split at a trigger (chosen).**

## Consequences

- "Always rebuildable" survives as *"rebuild in place is always safe"*; the
  stronger *"the file is disposable"* is retired until phase 2 lands.
- The guarding test makes the tenant tables' survival a stated invariant of
  `Rebuild`, so the next person changing its DELETE set learns why in a test
  failure instead of a production incident.
- Phase 2 is tracked as deferred debt with a named trigger, not an open-ended
  "later".
