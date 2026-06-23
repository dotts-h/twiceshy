# ADR-0016: Advisory-class records auto-promote via a diverse judge-panel (no repro)

- **Status:** Accepted (2026-06-20) — deciders: **horia** (directed "no human for
  this"; chose the judge-panel gate and the daily-review posture); claude proposed
  and authored. **Supersedes ADR-0013 §5** for the advisory class only; all other
  ADR-0013 sections stand.
- **Amended 2026-06-22 (#0071):** §7 added — the panel must not promote a
  *born-stale* advisory (an EOL runtime, or a `valid.until` already past). This is
  the promote-side mirror of #302's import-side staleness scoping; without it,
  panel-promoted EOL advisories became validated records that tripped the D2 guard
  and stuck ~36 validate PRs.
- **Amended 2026-06-23 (#0086):** the frontier (second-family) seat may run a
  **hybrid** judge — Gemini primary (off-pool; §5 already permits Gemini on public
  advisory data), Sonnet fallback consulted **only on a primary error** (free-tier
  Gemini exhausting its daily quota → 429), never on a primary reject. Off-pool on
  the happy path without the daily-quota stall a straight swap would cause; the
  pooled fallback keeps throughput up. See `judge.FallbackJudge`.
- **Related:** [ADR-0013](ADR-0013-closed-loop-autonomous-validation.md) (the
  closed loop this extends — §1 proof+judge, §2 veto window, §6 diversity/fail-safe,
  the standing rule *local LLM = drafter/flagger, never judge*); [ADR-0003 §4](ADR-0003-corpus-bootstrap-source-scope.md)
  (source-license / facts-only provenance the judge's license check reads);
  [ADR-0001 §3–4](ADR-0001-architecture.md) (the relevance floor + quarantine
  invariants this keeps intact). Epic #0027.

## Context

ADR-0013 removed the human *approver* for execution-provable records: a holding
broker attestation **plus** a diverse-model judge PASS auto-promotes
`quarantined → validated`. It deliberately **kept a human** for one class (§5):
conventions, prose lessons, and **OSV/GHSA advisories** — "no proof to stand on
and a higher poison risk."

That carve-out is now the binding constraint. The corpus is **92 quarantined / 9
validated**, and **86 of the 92 quarantined are OSV/GHSA advisories** — 93% of the
pile. The nightly validate driver (ADR-0013 §2, #0043) ran once and promoted
**zero**: every advisory returned *"no executable proof — left for a human (ADR-0013
§5)."* The engine is working exactly as designed and producing nothing, because the
class that dominates the corpus is the class §5 will never auto-promote. A
human-gated pile that large is, in practice, **never reviewed** — so the records
stay quarantined, and quarantined records are invisible to agents (the push channel
and the default pull path both filter `status = 'validated'`). The corpus grows and
serves nothing. That is the same "inert without a human in the loop" failure
ADR-0013 set out to kill — §5 just relocated it to the advisory class.

The §5 reasoning was *"no proof + high poison risk."* Re-examined for advisories
specifically, both premises are weaker than they read:

1. **The "proof" §5 wanted is the wrong proof for an advisory.** An advisory's
   claim is not behavioural ("this code path misbehaves") — it is a **factual
   transcription** ("package P, versions [introduced, fixed), has vuln V per a
   named upstream source"). There is nothing to *execute*; the analogue of proof is
   **fidelity to a trusted, public source** (OSV.dev / GHSA), which a judge can
   check directly against the record's `source_url` and version ranges.
2. **Poison risk is lower here, not higher, than for prose.** An advisory is
   structured, machine-imported from a single trusted feed, and low-interpretation
   — the failure mode is a *mis-transcription* (wrong package, wrong range, stale),
   not misleading *advice*. That is exactly what a judge that compares the record to
   its cited source catches.

What §5 got right and we keep: a single self-judging model is a monoculture trap,
and unproven records must not silently steer agents. The answer is not to drop the
gate — it is to make the gate a **panel** and keep ADR-0013's veto window.

## Options considered

- **A — keep ADR-0013 §5 (status quo).** Rejected: leaves 93% of the corpus inert
  and human-gated-in-name-only; reproduces the very failure ADR-0013 fixed.
- **B — serve quarantined advisories directly** (drop the `validated` filter for
  them). Rejected for the same reason ADR-0013 Option B was: removes the trust
  boundary; unjudged cards steer agents. The poisoning failure mode, not a fix.
- **C — auto-promote advisories on a single judge, no panel** (reuse the §1 judge
  seam, just waive the attestation). Rejected: §1's judge is *secondary* to a
  deterministic execution gate that bounds a hallucinated PASS to "mis-scoped but
  functionally-correct." Waive the attestation and the judge becomes the **sole**
  gate — a single model self-certifying unproven records is the monoculture trap
  ADR-0013 §6 names. One model is not enough when there is no execution anchor
  beneath it.
- **D — diverse judge-PANEL replaces the human approver for advisories (chosen).**
  For the advisory class only, promotion requires a **unanimous PASS from a panel of
  ≥2 judges of distinct model families** (gpt-oss:20b on the off-pool Ollama shim +
  gemini), each checking the advisory against its cited source; no attestation. The
  panel rides ADR-0013 §2's held-PR + veto window unchanged. Prose/conventions keep
  the §5 human. This restores the panel's independent-check property *without* an
  execution anchor — diversity does the work proof did in §1 — and keeps the human
  as overseer-not-gatekeeper, which is the stated goal.

## Decision

1. **A diverse judge-panel is the approver for advisory-class records.** A record
   is *advisory-class* iff it is an externally-imported vulnerability advisory —
   detected by a **vulnerability identifier** (`GHSA-`, `CVE-`, or `GO-` prefix) in
   its `symptom.error_signatures` (or fingerprints), the structural marker an
   advisory carries and a deprecation/codemod record does **not**. This is
   deliberately narrower than "imported + has a version range": importer-sourced
   deprecation records (e.g. `strings.Title`) also carry `author: twiceshy-importer`
   and an `applies_to` version range, yet they are *execution-provable* and stay on
   the ADR-0013 §1 proof+judge path (the drafter attaches a repro). Only records
   carrying a vuln id — a public advisory to transcribe, nothing to execute — route
   to the panel, never by free-text body match. For such a record, `quarantined → validated`
   requires a **unanimous approve from a panel of ≥2 judges whose model families
   differ** (anti-monoculture, ADR-0013 §6). There is **no broker attestation** in
   this path — the analogue of proof is the judge's fidelity check against the cited
   public source. Every member's verdict (model + decision + checks) is recorded in
   `provenance.promotion` for audit, exactly as §1 records its single verdict.

2. **The panel fails safe in every direction (ADR-0013 §6 holds).** Any member that
   errors, times out, garbles, or rejects yields **no promotion** — unanimity is
   required, so one dissent or one failure keeps the record quarantined. The panel
   is never bypassed: if fewer than the configured members are reachable, the record
   stays quarantined (a missing judge is a reject, not a skip). Each member is itself
   wrapped in the §F1 majority-vote (repeat-N) to absorb single-shot
   non-determinism, so a "member PASS" already means a stable majority for that
   model.

3. **The advisory checks are adapted, not invented.** The judge inspects the four
   ADR-0013 §1 dimensions, re-read for a no-repro advisory:
   *meaning* → the advisory is **faithfully transcribed** from its cited source
   (right vuln id, right package, right version range); *scope* → `applies_to`
   matches the source's affected ranges, not broadened; *license* → ADR-0003
   source-license/facts-only provenance is present and clean; *poison* → the record
   could not mislead an agent (e.g. a fixed-version that would flag safe code). An
   advisory judge-prompt (`AdvisorySystemV1`) carries these; it is the prose path's
   sibling, selected by class.

4. **The veto window is the human's seat, and the only human seat (ADR-0013 §2).**
   Promotions batch into one held PR; CI greens; the panel verdicts land in
   provenance; the PR self-merges only after the configured cooldown, during which a
   human may **veto** (close with a reason — that close is the audit trail) but is
   **never required to act**. For the advisory class the cooldown is tuned for a
   **daily** review cadence: the operator skims the held batch each day and may veto
   or — the forward direction — **enhance the judge skills / corpus**; no action
   means it goes live. "No human required, a human always allowed," now for
   advisories too.

5. **gemini sees public advisory data only — a hard privacy gate.** The gemini
   family's endpoint trains on inputs (free tier), so it is wired **exclusively** on
   the advisory path, whose content is public OSV/GHSA data. Prose, conventions, and
   any record carrying internal/sensitive content never reach gemini: they remain on
   ADR-0013 §5's human gate and, where judged, a local family. The class predicate
   in §1 is therefore also a **routing guard** — non-advisory records cannot enter
   the gemini panel by construction.

6. **Supersede-never-delete, git+CI boundary, relevance floor — all unchanged.**
   Every promotion is a CI-checked git commit, reversible by supersede (ADR-0001).
   The k≤3 hard cap and relevance floor (ADR-0001 §3–4) and the quarantine→push bar
   are untouched; this ADR changes *what may become validated*, not how validated
   records are retrieved or injected.

7. **A born-stale advisory is not promote-worthy — the panel is gated by the D2
   staleness check (#0071, companion to #302; amendment 2026-06-22).** Before the
   panel is consulted, an advisory whose runtime is already end-of-life (or whose
   `provenance.valid.until` is already past) is **held, quarantined**: `promote`
   consults the same end-of-life signal the D2 staleness doctor uses
   (`doctor.Staleness.WouldFlag`, the status-independent form of the
   validated-scoped guard). Rationale — #302 scoped the D2 guard to *validated*
   records so the importer could ingest EOL-runtime advisories as quarantined
   drafts (a draft is not "drift"); this §7 closes the **mirror gap on the promote
   side**: promoting such a draft manufactures a validated record the guard then
   flags, reding the very test that gates the validate PR (observed: ~36 stuck
   validate PRs, 2026-06-22). The gate **fails open** (an endoflife.date outage ⇒ no
   flag ⇒ promotion proceeds), matching the doctor's "no data ⇒ no flag" rule, with
   the deterministic guard test as the backstop. **Scope:** the advisory path only —
   the §1 proof+judge path is unchanged (a repro that holds on an EOL runtime is a
   separate question, not the stuck-PR cause).

## Consequences

- **Good.** The 86 advisories become eligible for autonomous promotion under an
  independent, diverse, auditable gate; the corpus stops being inert. The human role
  collapses to a **daily skim + enhance**, which is the posture horia asked for. The
  panel is a strictly stronger check than §1's single judge for the no-proof case.
- **Cost / latency.** Two model families × majority-N per advisory is more judge
  calls than §1; both families are **off the Anthropic weekly pool** (local
  gpt-oss + gemini), so the cost is latency and gemini quota, not Claude budget.
  Batched nightly, this is acceptable; the panel is an injectable seam, so N and
  membership are config.
- **Residual risk + backstop.** A panel can still unanimously bless a subtly
  mis-transcribed advisory. Backstops, in order: the per-member majority vote, the
  unanimity requirement, the **daily veto window**, supersede-on-discovery, and the
  ADR-0013 §3 outcome-feedback loop (a served advisory that misfires can be demoted
  /superseded). The first **Opus 4.8 quality review (tonight)** audits the new gate
  and the first promoted batch before this rides unattended.
- **Scope discipline.** This ADR moves **only** the advisory class off the §5 human
  gate. Conventions and prose lessons stay human-gated; revisiting them is a
  separate decision (a future multi-model prose panel), not licensed here.
- **Born-stale exclusion (#0071, amendment).** The panel no longer manufactures
  validated-but-immediately-stale advisories, so the validate PR stays green as the
  corpus grows across ecosystems. Cost: one endoflife.date lookup per product per
  run (memoized), made *before* the more expensive panel — a net saving on the EOL
  records it now skips. Residual: the gate fails open on a source outage, so the
  deterministic D2 guard test stays the backstop for the known-EOL cycles.
