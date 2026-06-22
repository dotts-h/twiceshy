# ADR-0020: Prose-class records auto-promote via a stronger, local-only judge panel

- **Status:** Proposed (2026-06-22) — decider: **horia** (directed "A — autonomous
  panel, live"); claude proposed and authored. **Supersedes ADR-0013 §5** for the
  **prose class** (conventions + non-advisory narrative lessons) — the last class on
  the §5 human gate. ADR-0016 already moved the advisory class; this completes the
  migration. All other ADR-0013 sections stand.
- **Related:** [ADR-0016](ADR-0016-advisory-class-panel-promotion.md) (the advisory
  panel this extends — and whose §Consequences explicitly deferred "a future
  multi-model prose panel" as "a separate decision," which this is);
  [ADR-0013](ADR-0013-closed-loop-autonomous-validation.md) §1 (proof+judge), §2 (veto
  window), §5 (the human gate this supersedes for prose), §6 (diversity/fail-safe; the
  "local LLM = drafter/flagger, never judge" rule, re-examined below);
  [ADR-0018](ADR-0018-session-retro-capture.md) (#0065 retro capture, which lands the
  prose traps this promotes); the #0011 ingestion safety gate (the content screen the
  panel makes mandatory). Epic #0064.

## Context

ADR-0016 moved the **advisory class** off ADR-0013 §5's human gate, because an advisory
is a low-interpretation *transcription* a diverse panel can check against a cited public
source. It deliberately **left prose human-gated**, naming the reversal for prose "a
separate decision (a future multi-model prose panel), not licensed here." This ADR is
that separate decision.

It is now binding. **#0065 (ADR-0018) session-retro capture** lands organic session traps
as `quarantined` records — and the majority are **pure prose**: a `convention`, a
narrative gotcha, a "don't do X" with no repro and no vuln id. They are **neither**
execution-provable (no repro → not the §1 path) **nor** advisory-class (no vuln id → not
the ADR-0016 panel). So they stay on the §5 human gate — and a human-gated pile is, by
ADR-0016's own argument, *"in practice never reviewed."* Enabling retro capture without a
prose-promotion path just grows an unreviewed quarantined pile that serves no agent: the
exact "inert without a human in the loop" failure ADR-0013 set out to kill, relocated one
more time. **#0065's captured traps are dead-weight until this is resolved** — the real
blocker for the value of epic #0064.

The §5 reasoning for prose was *"no proof + **higher** poison risk"* — higher than
advisories. Re-examined, that second premise **holds**: prose genuinely is higher poison
risk than an advisory. An advisory's failure mode is a *mis-transcription* (wrong
package/range) checkable against a source; prose's failure mode is **misleading advice** —
free-text, high-interpretation, with no external source to check fidelity against. So
unlike ADR-0016, we do **not** argue the risk is lower. We accept it is higher and answer
it the only way consistent with ADR-0013's thesis: not by keeping a never-reviewed human
gate, but by making the autonomous gate **strictly stronger than the advisory panel** and
keeping the human as overseer (the §2 veto window), not gatekeeper.

Two hard constraints shape the design:

1. **Privacy (ADR-0016 §5).** The gemini family's free-tier endpoint trains on inputs.
   Advisory content is public OSV/GHSA data, so gemini may judge it. **Prose may carry
   internal/sensitive content**, so the **gemini free-tier panel seat is excluded** for the
   prose class. Its place is taken by an **operator-designated, privacy-acceptable off-pool
   frontier family** — here the **Antigravity offload (`agy`)**, which the operator deems
   acceptable for prose content. The prose panel is therefore **gpt-oss (the off-pool local
   judge ADR-0016 already trusts) + `agy`** — two distinct families, neither the excluded
   gemini free tier.
2. **"Local LLM = drafter/flagger, never judge" (ADR-0013 §6) is honored unchanged.** The
   `localFamilies` denylist (the small on-box Ollama models — llama/codellama/qwen/nomic)
   stays **fully enforced**; no denylisted model judges. The prose panel's members are
   gpt-oss (already the advisory panel's blessed off-pool local judge) and `agy` (an
   off-pool frontier family), so §6's anti-self-judge guarantee is untouched. (An earlier
   draft of this ADR considered admitting a second *local* family to keep prose on-box; the
   operator chose `agy` instead, which keeps **both** cross-family diversity and the §6
   denylist — the cleaner option.)

## Options considered

- **A — prose-class judge panel, guardrails ≥ the advisory panel (chosen).** A diverse
  **local-only** panel (≥2 distinct local families, unanimous), a prose-specific prompt
  with the *poison* check foregrounded, a **mandatory clean content-screen**, a **longer
  veto cooldown** than advisories, and abstain-on-uncertainty. Supersede §5 for prose;
  keep the human as veto-not-gate.
- **B — committed lightweight human review of retro-draft PRs.** Rejected: it is the §5
  status quo with better intentions; ADR-0016 already showed a human-gated pile is never
  reviewed in practice. It does not remove the human from the loop (the project's stated
  goal) and it scales with reviewer attention, not with capture.
- **C — serve quarantined prose with a label.** Rejected (ADR-0013 Option B, ADR-0016
  Option B): removes the trust boundary; unjudged prose steers agents. The
  corpus-poisoning failure mode, not a fix. Listed only to be explicitly ruled out.

## Decision

1. **A diverse local-only judge-panel is the approver for prose-class records.** A record
   is *prose-class* iff it is quarantined, **not advisory-class** (no GHSA/CVE/GO id —
   those route to ADR-0016), and **not execution-provable** (no `guard.repros` — those
   route to ADR-0013 §1's proof+judge path; the #0026 drafter attaches a repro where it
   can). The residue — conventions and narrative traps with nothing to execute and no
   source to transcribe — is the prose class:
   `IsProseClass(rec) = !IsAdvisoryClass(rec) && !hasRepro(rec)`. Promotion requires a
   **unanimous approve from a panel of ≥2 judges whose model families differ** — gpt-oss
   (the off-pool local judge) + an operator-designated privacy-acceptable off-pool frontier
   family (`agy`); the gemini free tier is excluded for prose (2a). Every member's verdict
   (model + decision + checks) is recorded in `provenance.promotion`, exactly as §1 /
   ADR-0016 record theirs.

2. **The prose panel is strictly stronger than the advisory panel — five ways:**
   a. **Privacy gate, cross-family diversity preserved.** The gemini free tier is excluded
      two ways: the class predicate routes only prose to the prose panel (mirroring
      ADR-0016 §5), AND a **code guard rejects a gemini-family model on a prose judge at
      construction** (`NewModelJudge` errors when `Prose && family=="gemini"`) — so a
      misconfigured `TWICESHY_PROSE_PANEL_JUDGE_MODEL` cannot silently leak prose to a
      training endpoint, matching the §6 denylist's "rejected by construction" posture. The
      seat is the operator-designated `agy`, deemed privacy-acceptable for prose. So the
      panel keeps **cross-family** diversity (gpt-oss + agy) — comparable to the advisory
      panel's, not weaker — while the §6 local denylist stays fully enforced.
   b. **The poison check is foregrounded.** A prose-specific prompt (`ProseSystemV1`)
      re-reads the four ADR-0013 §1 dimensions for prose and makes *poison* the gating
      question: "could a competent agent, following this advice literally, be led to a
      WORSE action than doing nothing?" A prose record that cannot be shown harmless is
      rejected, not approved-by-default.
   c. **A clean content-screen is mandatory.** A prose record carrying ANY
      `provenance.security_flags` (the #0011 ingestion safety gate: secret/PII/harmful) is
      **held, never promoted** — the screen is a precondition to the panel, not advisory.
   d. **A longer veto cooldown.** Prose batches get a strictly longer §2 soak than
      advisories — more time for the daily human skim to catch a bad promotion before it
      serves.
   e. **Abstain-on-uncertainty.** A member that is unsure votes reject; unanimity over a
      high bar means a single hesitation holds the record. (The per-member §F1
      majority-vote still absorbs single-shot noise.)

3. **The four dimensions, re-read for prose.** *meaning* → the lesson is coherent,
   correct, and generalizable (not a one-off or a misread); *scope* → `applies_to` is
   accurate and **not over-broad** (over-generalization — "never use X" when X is fine in
   most cases — is prose's characteristic failure); *license* → ADR-0003 provenance is
   clean; *poison* (gating, 2b) → the advice could not mislead. An `AdvisorySystemV1`
   sibling (`ProseSystemV1`) carries these, selected by class.

4. **The panel fails safe in every direction (ADR-0013 §6 holds).** Any member that
   errors, times out, garbles, abstains, or rejects → **no promotion**. Fewer than the
   configured members reachable → quarantined (a missing judge is a reject, not a skip).
   The panel is never bypassed.

5. **The veto window is the human's seat, and the only one (ADR-0013 §2).** Prose
   promotions batch into a held PR; CI greens; verdicts land in provenance; the PR
   self-merges only after the **prose cooldown** (longer than advisory, 2d), during which a
   human may veto (close with a reason — the audit trail) but is never required to act. "No
   human required, a human always allowed," now for prose too.

6. **Privacy, supersede-never-delete, git+CI boundary, relevance floor, born-stale gate —
   unchanged.** Prose never reaches gemini (1, 2a). Every promotion is a CI-checked git
   commit, reversible by supersede (ADR-0001). The k≤3 cap + relevance floor (ADR-0001
   §3–4) and the quarantine→push bar are untouched. The ADR-0016 §7 born-stale gate (a
   `provenance.valid.until` already past → held) applies to prose too.

## Consequences

- **Good.** #0065's captured prose traps become eligible for autonomous promotion under an
  independent, diverse, auditable, *local* gate — the corpus stops accreting dead-weight,
  and epic #0064's capture half finally reaches agents. The human role stays "daily skim +
  enhance" (ADR-0016's posture), now across both unproven classes. §5 is fully retired;
  ADR-0013's "no human in the provable loop" thesis now covers the whole corpus.
- **Cost / risk posture.** This consciously **reverses ADR-0013 §5 for the highest-poison
  -risk class**, accepting that risk rather than relocating the never-reviewed-pile failure
  one more time. The reversal is bounded by guardrails *stronger* than the advisory panel
  (a higher bar for a higher risk) and by the backstop ladder: per-member majority vote,
  unanimity, the longer daily veto window, supersede-on-discovery, and the ADR-0013 §3
  outcome-feedback loop (a served prose card that misfires is demoted/superseded).
- **Residual risk + backstop.** A local panel can still unanimously bless
  plausible-but-misleading prose, and unlike an advisory there is **no cited source** to
  check fidelity against — the panel judges the advice on its own coherence + harmlessness.
  This is the genuinely new risk this ADR takes on. Backstops, in order: the foregrounded
  poison check, the mandatory content-screen, the longer veto window, outcome-feedback
  demotion, and an **Opus 4.8 quality audit of the prose gate + its first promoted batch
  before it rides unattended** (mirroring ADR-0016's first-batch audit). A **prose gold
  set** (positive + adversarial-poison cases) gates the prompt in `judge-eval`, exactly as
  the advisory gold set does, so the panel's separation is measured, not assumed.
- **Diversity is gemini-free, not narrowed.** Excluding the gemini free tier for privacy is
  covered by the operator-designated `agy` family, so the prose panel keeps cross-family
  diversity (gpt-oss + agy) **and** the §6 denylist — no safety axis is traded away. The
  panel membership is an injectable config seam, so a third family can be added without an
  ADR if the two prove too correlated. (Operator responsibility: the `agy` endpoint's
  data-handling must remain acceptable for prose; if that ever changes, the seat reverts to
  another privacy-acceptable family — not the gemini free tier.)
- **Scope discipline.** This ADR moves **only** the prose class. It does not change
  retrieval, the push gate, or how validated records are served; it changes *what may
  become validated*. The retro **deploy glue** (the SessionEnd hook + the retro-intake
  unit) remains separately tracked.
