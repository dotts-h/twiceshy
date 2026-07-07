# ADR-0031: Alpha write-path policy is a single seam, not per-handler ifs

- **Status:** Accepted (2026-07-07; horia approved the 2026-07-07 architecture
  review's proposal; blocks #0128 write-path enablement, with ADR-0032).
- **Related:** ADR-0030 phase 2 (the enablement this gates), #0128 (the
  hardening this completes and centralizes), ADR-0013 §3 (the adapt/demote
  loop whose corroboration counting assumes author identity means something),
  #0118 (per-origin trust tiers keyed off `provenance.source.author`),
  ADR-0032 / #0135 (the quota debit these policies charge), issue 0136.

## Context

#0128 hardened the write path for untrusted alpha tok_ tenants: forced
`alpha:<token_id>` origin stamping, tighter size caps, forced PII redaction,
fail-closed secret rejection, per-tool contribution quotas. The 2026-07-07
architecture review found the full package applied **only to
`record_experience`**; the policy exists as inline `if alpha {...}` blocks
per handler, and every other write surface got a different subset:

- `report_outcome` / `report_issue`: quota only. `args.Author` flows
  **verbatim** into `provenance.source.author` (`report.go:87`) and the
  spooled issue. Since dispute counter-records feed the adapt loop, whose
  `DisputeThreshold` counts *independent* reports as corroboration, one
  hostile token can fabricate N "independent" authors and walk a validated
  record toward `disputed` — and can spoof importer/trusted origins that
  #0118's trust tiers key off. No alpha size caps; secrets quarantine
  instead of rejecting (a different posture from record_experience for the
  same tenant class).
- `confirm_helpful`: no contribution bound at all — reinforcement-counter
  inflation at rate-limit speed.
- `POST /retro`: no alpha posture decided — any tok_ token can spool 256 KiB
  transcripts per call into the off-pool analyzer queue.

The structural cause, not the individual gaps, is the decision here: policy
re-decided (or forgotten) per handler is the same accretion pattern that
produced #0131.

## Decision

Alpha write-path policy is declared **once** and applied uniformly; a
completeness test makes "a write tool skipped the policy" a test failure,
not a review catch.

1. **One declaration point** (`internal/server/alpha_policy.go`): the
   per-tool contribution quotas and the shared alpha posture helpers —
   origin stamping (`alpha:<token_id>` forced; the caller-supplied author is
   preserved only as a display note in the free-text body, the
   record_experience pattern), alpha size caps, and the fail-closed secret
   posture (reject, never quarantine, for alpha input).
2. **`report_outcome` and `report_issue` adopt the full posture:** stamped
   origin, alpha caps on their free-text fields, secret-shaped content
   rejected outright. Their quotas keep the #0128 values.
3. **`confirm_helpful` gets a contribution quota** (50/day per token): it
   mutates durable reinforcement state, so it is a write for quota purposes,
   even though it stamps no provenance.
4. **`POST /retro` is operator-only for the alpha** (403 for tok_ tenants):
   ADR-0030 phase 2 opens `record_experience`/`report_outcome` only, and
   retro spools cost directly into the off-pool analyzer. Opening retro to
   tenants is a future decision with its own quota, not a default.
5. **A completeness test** enumerates the server's write surfaces and fails
   if any is missing from the policy declaration — the guard that makes the
   seam stick for the next tool.

## Options considered

- **Leave per-handler ifs, just fix the gaps — rejected:** fixes today's
  three handlers and re-opens the same hole on the fourth; no structural
  guarantee, which is the actual defect.
- **A generic policy middleware wrapping every MCP tool — rejected:** size
  caps and secret scans are argument-typed (RecordArgs vs ReportArgs); a
  generic wrapper can't inspect typed args without per-tool adapters, which
  reintroduces the per-tool scatter one layer down. The declaration table +
  shared helpers + completeness test gets the guarantee without fighting the
  SDK's typed handlers.
- **Deny alpha writes everywhere except record_experience — rejected:**
  report_outcome is half the flywheel (ADR-0030 phase 2 names it); the
  moderation pipeline exists to admit it safely.

## Consequences

- `provenance.source.author` becomes trustworthy by construction for every
  tok_ write: adapt-loop corroboration counts one token as one voice
  (sybil pressure moves to signup, which is IP-capped), and #0118 trust
  tiers have a real key.
- Alpha posture is consistent: an alpha tenant gets the same secret
  handling, caps, and stamping on every tool; operator behavior is
  unchanged everywhere.
- CONTRACTS.md rows for `report_outcome`, `report_issue`, `confirm_helpful`
  and `POST /retro` gain the alpha-tenant posture (a deliberate, recorded
  contract change — this ADR).
- Adapting the demote loop to *group* disputes by stamped origin (rather
  than trusting author strings from any channel) is follow-up work in
  `internal/promote`, out of this ADR's server-side scope.
