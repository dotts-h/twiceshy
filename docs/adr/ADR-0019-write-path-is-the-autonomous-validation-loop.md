# ADR-0019: The write path is the autonomous validation loop + direct quarantined import — superseding ADR-0008 §2–4

- **Status:** Proposed (2026-06-22) — claude proposes, **documenting already-shipped
  reality** for horia to accept. The superseding *decisions* (ADR-0013 closed loop,
  ADR-0016 advisory panel) were horia-directed and are Accepted; this ADR only trues
  the ADR log to what those, plus the scheduled importer, actually built.
- **Supersedes:** [ADR-0008 §2–4](ADR-0008-write-path-persistence-is-a-cli-concern.md)
  (the `twiceshy propose` + `internal/publish` / `publish.Publisher` per-record write
  path). **Preserves ADR-0008 §1** (the still-true kernel — see Decision 3).
- **Related:** [ADR-0013](ADR-0013-closed-loop-autonomous-validation.md) (the
  promote/adapt loop that became the write path), [ADR-0016](ADR-0016-advisory-class-panel-promotion.md)
  (advisory-class panel promotion), [ADR-0001 §6](ADR-0001-architecture.md) (the
  quarantined-only / PR-as-trust-boundary invariant — **locked, unchanged**),
  [ADR-0011](ADR-0011-corpus-growth-and-validation-engine.md)/#0022 (the scheduled importer).

## Context

ADR-0008 (Accepted, horia, 2026-06-17) decided the write path would be a trusted CLI
subcommand `twiceshy propose` backed by an `internal/publish` package exposing a
`publish.Publisher` seam, opening **one quarantined PR per record**, so the
network-facing MCP server never holds forge credentials.

**That §2–4 implementation was never built and has been overtaken.** Verified against
the tree (2026-06-22): there is no `propose` subcommand in the `cmd/twiceshy` dispatch,
no `internal/publish` package, no `Publisher` symbol anywhere, and no `Publisher` row
in CONTRACTS.md. What shipped instead, under newer **Accepted** ADRs:

1. **Autonomous status transitions in place (ADR-0013, #0029).** `twiceshy promote` /
   `adapt` walk the corpus and flip `quarantined → validated` (or demote) directly,
   gated by a holding broker attestation + a diverse-model judge — recording the
   verdict in `provenance.promotion`. No per-record `propose`/PR; the status change is
   committed by the run.
2. **Advisory-class panel promotion (ADR-0016).** OSV/GHSA advisories promote via a
   diverse judge panel (no repro), same in-place model.
3. **Direct quarantined import at the corpus level (ADR-0011/#0022).** The scheduled
   importer commits *batches* of new quarantined records and opens **one corpus-level
   PR per batch** (e.g. `corpus(osv-live): 433 new quarantined records`) — the trust
   boundary is that corpus PR + CI, not a per-record `Publisher`.

The repo's hard rule is **"supersede, never delete — records and ADRs alike"**
(CLAUDE.md), and it is followed elsewhere (ADR-0009 supersedes the sqlite-vec wording;
ADR-0016 supersedes ADR-0013 §5). ADR-0008 §2–4 is the one Accepted decision left
contradicting reality with no superseding record — which makes the ADR log untrustworthy
as a statement of what is true. This ADR closes that gap.

## Decision

1. **Record the write path as it is.** Persistence of *new* records is the corpus-level
   import PR (a batch of `quarantined` records, CI-gated); promotion/demotion is the
   ADR-0013 promote/adapt loop (judge-gated, in place) plus the ADR-0016 advisory panel.
   The unit of human trust is the **CI-checked git PR** (corpus batch, or the held
   promote/adapt batch under the ADR-0013 §2 veto window) — exactly ADR-0001 §6's
   PR-as-trust-boundary, reached by a different mechanism than ADR-0008 §2 imagined.

2. **`twiceshy propose` + `internal/publish` / `Publisher` are withdrawn.** They are not
   on the roadmap; the per-record-PR model is superseded by the batch/loop model above.
   Anything needing them should instead use `promote`/`adapt` or the importer path.

3. **ADR-0008 §1 stands — the security kernel is unchanged and locked.** The MCP server
   is read-only over the network and **never holds a forge/push credential**;
   `record_experience` / `report_outcome` / `report_issue` are **propose-only** (they
   return a draft or enqueue to a spool, never push). The threat model ADR-0008 cited
   (MINJA query-only poisoning → don't give the untrusted-input service push power) is
   why this kernel is preserved verbatim. The scheduled importer and the promote/adapt
   loop run in the **trusted local/CI context** where git credentials already live —
   precisely ADR-0008's principle, just applied to the loop/import rather than `propose`.

## Consequences

- **Good.** The ADR log again matches the code: the write path has exactly one current,
  recorded description. Onboarding and audit can trust ADR-0008's status line.
- **Unchanged security posture.** No new privilege: the network service still holds no
  push creds (ADR-0008 §1); writes happen in trusted contexts behind a CI-gated PR.
- **On accept:** flip ADR-0008's header to `Superseded by ADR-0019` (its §1 carried
  here), and update `docs/adr/README.md`. Until then ADR-0008 carries a drift note
  pointing here, so the contradiction is at least visible.
- **Scope.** This is documentation-trueing of shipped, Accepted decisions; it introduces
  no new behavior and no code change.
