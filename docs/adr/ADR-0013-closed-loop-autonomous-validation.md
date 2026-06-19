# ADR-0013: Closed-loop autonomous validation — proof + a diverse-model judge replace the human approver for execution-provable records

- **Status:** Proposed (deciders: claude proposes; **horia to ratify** — this
  refines load-bearing decisions ADR-0001 §7, ADR-0008, and ADR-0010, so it needs
  sign-off, like ADR-0011).
- **Related:** [ADR-0001 §7](ADR-0001-architecture.md) (doctors are delta-only,
  never whole-store rewrites — locked); [ADR-0008](ADR-0008-write-path-persistence-is-a-cli-concern.md)
  (git/PR is the trust boundary); [ADR-0010](ADR-0010-doctors-build-d2-defer-the-rest.md)
  (doctors propose-only; D4 deferred until retrieval increments `provenance.usage`);
  [ADR-0011](ADR-0011-corpus-growth-and-validation-engine.md) (the engine — §4
  sandbox, §8 drafter); [ADR-0012](ADR-0012-cicd-trust-posture-and-runner-isolation.md)
  (the self-merge gate this rides). Standing rule: **local LLM = drafter/flagger,
  never judge.** Epic #0027.

## Context

The engine built under epic #0015 now **proves** repros (the gVisor broker #0018,
the revalidate doctor #0020) and **drafts + attaches** them (#0026). But two facts
make that machinery produce *dead weight* whenever a human isn't standing by:

1. **Promotion is human-gated.** Records are born `quarantined`; flipping to
   `validated` is a human PR (ADR-0010: doctors are propose-only; `server/record.go`:
   *"A record is NEVER stored as validated here — human review on the PR is the
   trust boundary."*).
2. **Quarantined records are invisible to agents.** The default pull path filters
   `r.status = 'validated'` (`internal/index/index.go` `appendStatusFilter`), and
   quarantined records never enter the push channel (`server/push.go:61`).

So a repro the broker just *proved* helps no consuming agent until a human promotes
it — which contradicts twiceshy's premise: a service that **enhances LLM workflows
automatically** and **learns from experience**. Worse, there is no reverse channel:
when a served lesson **misfires in practice**, nothing feeds that back, so the
corpus can't *adapt* — it's a write-once archive, not an experience that updates.

The intent of #0015 was "validated means we ran it and it holds." The execution
gate already delivers that *proof*. What still pins a human in the loop is
*judgment* — and judgment, per the project's own standing rules, can be a model.

## Options considered

- **A — keep the human approver for all promotions** (status quo). Rejected: the
  engine's output is inert without a human in the loop; fails the premise.
- **B — serve quarantined records too** (drop the `validated` filter). Rejected:
  removes the trust boundary entirely — unproven, unjudged cards would steer agents.
  This is the corpus-poisoning failure mode, not a fix.
- **C — auto-promote on a bare green attestation, no judge.** Rejected: *"a gate is
  a lead, not a verdict"* (the cheap-executor bake-off lesson, REGRESSIONS) — a
  repro can pass for the **wrong reason** or capture a **mis-scoped** lesson and
  still go green. Self-validating with no independent check is a monoculture trap.
- **D — replace the human *approver*, not the git/CI boundary (chosen).** For
  execution-provable records, the approver becomes **proof + a diverse-model
  judge**; add a **gated outcome-feedback** channel for the reverse direction
  (demote/supersede only on execution-backed counter-evidence). Non-provable
  records keep a human. Distinguishes *proof* (already automatic) from *judgment*
  (a model can do it) while preserving git history, CI integrity, and
  supersede-never-delete as the audit and safety net.

## Decision

1. **Proof + a diverse-model judge is the approver for execution-provable
   records.** For deprecation/codemod/behavioural records that carry a repro, a
   **holding broker attestation** (#0018/#0020) **plus a PASS from a `Judge` seam**
   auto-promotes `quarantined → validated`. The judge is a **frontier model,
   diverse from the drafter** (a different family — cf. `ask-gemini`, "the diverse
   reviewer"); the **cheap local model is forbidden as judge** (standing rule). The
   judge checks what a green gate cannot: does the proof actually capture the
   *intended, correctly-scoped* lesson; is it license-clean; could it mislead a
   future agent (poison).

2. **Git + CI remain the boundary and the audit trail — only the human *approver*
   is removed.** Promotion flows through the ADR-0012 self-merge mechanism (a bot
   opens a promotion PR, CI greens, it self-merges); the record's `provenance`
   carries the attestation id + the judge verdict. We do **not** let a process
   write `validated` to the store unaudited — every promotion is a git commit,
   schema-checked by CI, and reversible by supersede.

3. **Closed-loop outcome feedback (the reverse direction).** A consuming agent
   reports a failure via a new MCP `report_outcome` tool. **A report is gated
   counter-evidence, not a verdict.** twiceshy turns it into a repro and re-runs the
   record's original repro **plus** the counter through the broker:
   - lesson's claim no longer holds, **or** the counter reproduces → the diverse
     judge approves a **demotion to `stale`** or a **superseding corrected record**;
   - it does **not** reproduce → at most `applies_to` is tightened (a scope/near-miss
     fix) — **a misapplied lesson never demotes a correct card.**
   Supersede-never-delete (ADR-0001) holds throughout.

4. **Usage is the reinforcement signal.** Retrieval increments `provenance.usage`
   (`retrieved`, `last_hit`; `confirmed_helpful` from a positive outcome report),
   unblocking ADR-0010's deferred **D4 lifecycle** doctor and giving the loop a
   decay/reinforce signal distinct from execution.

5. **Non-execution-provable records stay human-gated.** Conventions, prose lessons,
   and OSV-vulns-by-version-range have **no proof to stand on** and a higher poison
   risk, so they keep the human approver (or a future multi-model judge panel). The
   claim is **"no human in the *provable* loop," not "no human ever."**

6. **Diversity is mandatory and the judge is an injectable seam.** The judge must
   be a different model family than the drafter (anti-monoculture); its endpoint is
   stubbed in tests (no network in CI), like the embedder and endoflife seams. A
   judge outage **fails safe**: no verdict → the record stays quarantined; nothing
   is ever auto-promoted without a recorded PASS.

## Consequences

- **twiceshy finally lives its name.** Proven lessons go live on their own; misfires
  feed back and correct the corpus — learning from experience with no human in the
  provable loop. The #0026 output (exp-0043/0045) stops being dead weight.
- **Refines, does not abolish, ADR-0008/0010/0001 §7.** The git/PR/CI boundary
  stays; the *human approver* is replaced by proof+judge **for the provable class
  only**. ADR-0010's "propose-only" gains an "apply-when-proven-and-judged" path;
  doctors remain delta-only (per-record promotion, never a whole-store rewrite).
  ADR-0010 and ADR-0011 get a forward-link to this ADR.
- **New operational dependency:** an external diverse-model endpoint for the judge,
  **off the Anthropic pool** ([[llm-offload-stack]]). Injectable + stubbed; an
  outage blocks auto-promotion (fail-safe), never bypasses it.
- **New attack surface — the `report_outcome` channel.** Mitigated: reports are
  execution-gated counter-evidence (never direct mutations), pass the same content
  screen as ingest (#0011/#0019), and are rate-limitable. The worst a hostile
  report can do is *propose* re-validation work, which the gate adjudicates.
- **Monoculture residual.** LLM-drafted, LLM-judged, LLM-consumed. Reduced — not
  eliminated — by the execution gate + model diversity; revisit if drift appears
  (a periodic human or third-model spot-audit of auto-promotions is the escape hatch).
- **Multi-tenant raises the stakes.** Autonomous promotion makes the prepare-phase
  egress and the `/work` disk cap (#0025) **hard preconditions** for multi-tenant
  (epic #0010); the single-tenant brain is unaffected.
