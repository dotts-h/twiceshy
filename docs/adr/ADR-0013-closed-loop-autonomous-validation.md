# ADR-0013: Closed-loop autonomous validation — proof + a diverse-model judge replace the human approver for execution-provable records

- **Status:** Accepted (2026-06-19) — deciders: **horia** (ratified the direction
  and the veto-window safety); claude proposed. Refines load-bearing decisions
  ADR-0001 §7, ADR-0008, and ADR-0010 (forward-linked below).
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
   future agent (poison). **The trust anchor is the deterministic execution gate,
   NOT an LLM** — the judge is a *secondary* filter that only ever sees records
   that already ran fail-pre / pass-post, so a hallucinated PASS is bounded to "a
   functionally-correct but mis-scoped or misleading lesson," never arbitrary code.
   Proof covers *behaviour*, not *intent / safety-of-advice*; the poison check is
   therefore **best-effort**, backstopped by the veto window (§2), monitoring (§7),
   and the outcome-feedback loop (§3).

2. **Git + CI remain the boundary and the audit trail — only the *required* human
   approver is removed; a human can still veto (the held queue).** Promotion flows
   through the ADR-0012 self-merge mechanism **with a soak window**: a bot opens a
   promotion PR (this *is* the held queue), CI greens, the attestation id + judge
   verdict land in `provenance`, then the PR **self-merges only after a cooldown**
   (config, default ~48h) during which a human may skim and **veto** (close it with
   a reason — that close *is* the "why it was blocked" audit trail) — but is **never
   required to act.** No human action → it goes live; one bad card → a human catches
   it in the window. This is the synthesis the goal demands: *no human required,
   but a human always allowed* for oversight. Batched promotions share one PR so the
   skim is cheap. We do **not** write `validated` unaudited — every promotion is a
   git commit, CI-checked, reversible by supersede.

3. **Closed-loop outcome feedback (the reverse direction).** A consuming agent
   reports a failure via a new MCP `report_outcome` tool. **A report is gated
   counter-evidence, not a verdict.** twiceshy turns it into a repro and re-runs the
   record's original repro **plus** the counter **in the same hardened broker** (the
   gVisor sandbox — not a prose content screen — is what contains untrusted
   shell/env in counter-evidence):
   - claim no longer holds, **or** the counter reproduces → the diverse judge
     approves a **demotion to `stale`** or a **superseding corrected record**;
   - does **not** reproduce → **no silent drop**: independent non-reproducing
     reports *accumulate*, and past a threshold the card is flagged `disputed` and
     escalated — this is the deliberate cover for the non-deterministic / prod-only
     failures the sandbox cannot reproduce (see Threats);
   - any `applies_to` narrowing is **judge-gated, reversible, and needs
     corroboration** — one report cannot stealth-neuter a good card.
   The channel is **authenticated, rate-limited, and budget-capped** so it cannot be
   a DoS on the broker/judge. Demotions to `stale` are the safe direction (stop
   serving) and apply immediately; a *superseding* record adds served content and
   rides the §2 soak. Supersede-never-delete (ADR-0001) holds throughout.

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

7. **Guardrails are part of the decision, not an afterthought.** Autonomous
   promotion ships with: **(a) anomaly monitoring** — promotion/demotion rate +
   pattern alerts, so a judge that suddenly starts approving everything is caught
   ("who judges the judge"); **(b) an emergency stop** — one switch halts all
   auto-promotion (records pile up quarantined — fail-safe); **(c) budget caps** on
   the broker/judge runs a report can trigger. Tracked as issue #0033.

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
  execution-gated counter-evidence (never direct mutations); the counter-repro is
  contained by the same gVisor sandbox (prose is content-screened, #0011/#0019);
  the channel is authenticated, rate-limited, and budget-capped. The worst a hostile
  report can do is *propose* re-validation work, which the gate adjudicates.
- **Monoculture residual.** LLM-drafted, LLM-judged, LLM-consumed. Reduced — not
  eliminated — by the execution gate + model diversity; revisit if drift appears
  (a periodic human or third-model spot-audit of auto-promotions is the escape hatch).
- **Multi-tenant raises the stakes.** Autonomous promotion makes the prepare-phase
  egress and the `/work` disk cap (#0025) **hard preconditions** for multi-tenant
  (epic #0010); the single-tenant brain is unaffected.

## Threats and residual risks (diverse-model review, 2026-06-19)

A skeptical second-model pass (gemini, off-pool — dogfooding the very judge this
ADR proposes) surfaced these; recorded honestly rather than waved away:

- **Available-but-compromised judge = fail-*unsafe*.** "Fail-safe" above covers an
  *outage*; a judge that is up but subtly wrong/poisoned still approves. This is the
  sharpest residual. Layered cover, not a cure: model **diversity** (one compromised
  model ≠ both), the **veto window** (a human can catch it), **anomaly monitoring**
  (§7), and the **outcome-feedback loop** (a bad promotion that misfires gets
  reported → demoted). Accept consciously for single-tenant; reassess for §5/0010.
- **The sandbox is not production.** Proof in the broker is necessary, not
  sufficient — non-deterministic / environment / timing-specific failures pass the
  gate yet break in a real agent. The **accumulating non-reproducing reports →
  `disputed` escalation** (§3) is the deliberate cover; the gate alone is not relied
  on to be complete.
- **Rests on the broker's integrity.** The whole engine assumes the gVisor broker
  (ADR-0011 §4 / #0018) cannot be tricked into attesting a false proof. That is a
  pre-existing foundational assumption this ADR inherits, not one it introduces — but
  it is now load-bearing for *promotion*, not just reporting.
- **`report_outcome` DoS / stealth-neuter** — addressed in §3 (auth + rate-limit +
  budget cap; `applies_to` narrowing judge-gated + reversible + corroborated).
- **Accountability is not "the system."** Every auto-promotion is a git commit
  carrying its attestation + judge verdict; the accountable party is the maintainer
  who ratified this ADR and owns the audit trail — not an anonymous pipeline.
- **A formal LLM threat model** (prompt-injection of judge/drafter, adversarial
  inputs to the gate) is **required before multi-tenant** (epic #0010), where
  untrusted parties can submit; bounded today (only claude + horia submit).
