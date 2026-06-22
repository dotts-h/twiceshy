# ADR-0018: Session-retro capture — a `SessionEnd` hook spools the transcript; an off-pool analyzer drafts quarantined traps

- **Status:** Proposed (2026-06-22) — claude proposed; horia ratifies. Implements the
  headline child (#0065) of epic [#0064](../issues/0064-epic-agent-native-feedback-loop-capture-submit-measure.md).
- **Related:** [ADR-0013](ADR-0013-closed-loop-autonomous-validation.md) (the quarantine →
  judge → PR ladder this feeds; standing rule *local LLM = drafter/flagger, never judge*);
  [ADR-0008](ADR-0008-write-path-persistence-is-a-cli-concern.md) (git/PR is the write trust
  boundary — the analyzer drafts, it never promotes); [ADR-0001 §6](ADR-0001-architecture.md)
  (quarantined records never reach the push channel); the `report_outcome → spool →
  intake-reports` lifecycle (ADR-0013 §E1, `internal/spool`) this mirrors; the
  `DATA-not-instructions` envelope (#0012, `internal/server/render.go`).

## Context

twiceshy's agent write substrate is already built and secure (`record_experience`,
`report_outcome`, `confirm_helpful` — all quarantined, screened, judge-gated, authenticated,
DATA-enveloped). The gap epic #0064 names is not the substrate; it is that **agents reliably
fail to write back the traps they solve.** Pull is self-targeting (being stuck *is* the
trigger); submission has no intrinsic trigger and zero local payoff at the moment it would
fire, so it never fires. Observed directly: a real session found multiple traps and submitted
none. So the corpus grows only via the OSV importer (homogeneous advisories) and manual
dogfooding — it does not capture the **organic** traps agents actually hit.

Two hard constraints rule out the obvious fixes. A per-prompt *"remember to submit!"* nudge
becomes noise and gets disabled — exactly why the push hook was deferred (exp-0622). And hooks
inject context; they **cannot force a tool call.** So the capture must not depend on agent
volition at all.

## Options considered

- **A — a "remember to submit" nudge / reminder hook.** Rejected: indiscriminate hooks are
  noise and get turned off; and a hook cannot force the `record_experience` call anyway. Depends
  on the very volition that is missing.
- **B — the agent self-summarizes at session end and submits via `record_experience`.** Rejected:
  still volition-dependent (the model has to choose to author), burns the agent's own (Anthropic)
  pool to author lessons, and trusts the agent to self-report — the thing that empirically does
  not happen.
- **C — `SessionEnd` hook ships a bounded transcript to a server-side spool; an off-pool
  analyzer drains the spool and drafts quarantined traps through the existing ladder (chosen).**
  Server-side automation sidesteps "can't force a tool call": capture depends on *nothing the
  agent does*. The hook is dumb, deterministic, and fail-open.
- **D — analyze inline in the endpoint (synchronous).** Rejected: an expensive off-pool LLM call
  on the request path is a DoS surface and slows session exit. The spool-then-drain split is the
  proven `report_outcome` shape and keeps the edge thin.

## Decision

1. **Capture at the lifecycle seam, server-side — never agent volition.** A Claude Code
   **`SessionEnd`** hook (the once-per-session seam; `Stop` is per-response and too frequent)
   ships the session transcript to twiceshy. The hook is **fail-open** (any error → exit 0,
   never blocks the agent) and **dumb** (no model call client-side).

2. **Thin edge, fat driver — mirror `report_outcome → spool → intake`.** The hook POSTs to a
   raw **`POST /retro`** endpoint that does only two cheap things: **screen** the payload for
   secrets and **spool** it (atomic enqueue, `internal/spool`), then returns immediately. A
   separate driver — **`twiceshy retro-intake`**, scheduled like the other drains — runs the
   expensive **off-pool analyzer** over spooled transcripts. The endpoint never blocks on the
   model; the analysis is off the request path.

3. **Payload shape = a bounded raw transcript; the intelligence is server-side.** Not a
   model-authored summary (option B's trap). The hook tail-bounds the transcript to fit the
   inherited request-body cap and sends it verbatim. Keeping the hook deterministic means the
   *only* model in the loop is the off-pool analyzer, which is injectable, stubbed in tests, and
   sees the transcript framed as DATA.

4. **Reuse the quarantine ladder; add no new write path.** Each extracted trap becomes an
   `ingest.Draft` and goes through `ingest.Prepare` → a **quarantined** `*record.Record` →
   `writeRecord` → PR, exactly like the importer and `intake-reports`. Promotion to `validated`
   still requires the normal ladder (a human PR, or proof + a diverse judge per ADR-0013). **The
   analyzer drafts; it never promotes** — honoring *local LLM = drafter, never judge*. Its blast
   radius is bounded to "a quarantined draft a human/judge will vet," never a served card.

5. **Precision rides the existing dedup gate + quarantine, not a new heuristic.**
   `ingest.Prepare` already drops `Known` re-discoveries against the live corpus (and the driver
   dedups within a batch); quarantine + the PR/judge gate keep noise out of the served set. The
   analyzer prompt is conservative — extract only clear, novel, generalizable traps. Low homelab
   volume makes a heavy per-session off-pool pass affordable. (Calibration of the prompt's
   precision is deferred, like the push gate's.)

6. **Security is inherited, not rebuilt.** LAN-only; bearer required; rate-limited; 256 KiB body
   cap — all from the existing middleware. The transcript is **screened at the edge** before it
   is spooled: a `secret`-category finding **refuses the spool** (fail-closed — a secret never
   lands on disk), while `harmful-code`/`pii` findings (expected in a coding transcript: private
   IPs, shell snippets) do not. The analyzer wraps the transcript in a **`BEGIN/END SESSION
   TRANSCRIPT` DATA envelope** with breakout-neutralization (the analyzer is itself
   prompt-injectable). `ingest.Prepare` screens again at intake (defense in depth). Everything
   extracted is **quarantined**.

7. **The analyzer is an injectable, fail-safe seam — like the judge.** `retro.Analyzer` is an
   interface with a network-free `StubAnalyzer` (drives tests; no model in CI) and a
   `ModelAnalyzer` off-pool edge wired by env (reusing the `judge.ModelJudge` HTTP-to-shim
   transport idiom). An analyzer outage or error **leaves the transcript queued** for retry —
   never a partial write, never a dropped session, never a crash.

8. **Scope now = trap extraction; the used-vs-ignored signal is deferred.** This ADR/PR ships
   the capture spine (hook → endpoint → spool → driver → quarantined drafts). The second half of
   #0065 — joining the transcript against #0067's decision log to score *which served/pushed
   cards were used vs ignored* — is filed as a follow-up. #0065's charter explicitly blesses
   shipping the extraction half independently.

## Consequences

- **twiceshy finally captures the traps agents actually hit**, not just OSV advisories and hand
  dogfooding — the strategic corpus-value gap #0064 names. Dogfooding our other apps against
  twiceshy now produces durable corpus growth with no human in the capture loop.
- **New attack surface — `/retro` + an LLM analyzer over untrusted transcript text.** Mitigated:
  screen-at-edge refuses secrets before they are stored; the DATA envelope frames the transcript
  as data, not instructions; the analyzer's only output is *quarantined drafts* a human/judge
  vets (bounded blast radius — never code execution, never promotion); auth + rate-limit +
  body-cap are inherited; LAN-only.
- **New operational dependency — an off-pool analyzer endpoint** (reuses the judge stack on the
  Ollama VM). Injectable + stubbed; an outage queues transcripts (fail-safe), never bypasses or
  drops.
- **Privacy/token trade-off accepted for single-tenant.** A bounded raw transcript is shipped
  (richer than a summary, cheaper than burning the agent's pool to self-author). Client-side +
  edge screening reduce secret leakage; the residual full-transcript privacy exposure is accepted
  on a single-tenant LAN and **flagged for epic #0010** (multi-tenant needs a formal transcript-PII
  + LLM threat model, inherited from ADR-0013).
- **Precision residual.** Auto-extraction is noisy; the dedup + quarantine + PR/judge gate bounds
  it, but a flood of low-value drafts is possible. Monitored via the PR queue; the conservative
  prompt and the `-limit` on the drain are the throttles. Prompt calibration is deferred.
- **Refines #0064; touches no locked retrieval/promotion ADR.** The analyzer never promotes, so
  the trust boundary (ADR-0008/0013) and the validated-only push invariant (ADR-0001 §6) are
  untouched.

## Threats and residual risks

- **Prompt-injection of the analyzer.** The transcript is untrusted and an LLM analyzer is
  injectable. Cover: the `BEGIN/END SESSION TRANSCRIPT` envelope + breakout-neutralization, and
  the structural bound that the analyzer's output is only quarantined drafts gated by the PR/judge
  ladder — a successful injection yields at worst a misleading draft a reviewer rejects, never
  arbitrary code or a promotion.
- **Secret exfiltration via a transcript.** Screened client-side (before it leaves the machine)
  and refused at the edge on any `secret` finding (fail-closed); LAN-only. Residual: a novel
  secret shape the screen misses — bounded by LAN-only + quarantine + human PR review.
- **DoS via large/frequent transcripts.** Bearer + rate-limit + 256 KiB body cap (inherited); the
  expensive analysis is off the request path and bounded by the drain's `-limit`.
- **Multi-tenant raises the stakes.** A formal LLM threat model and transcript-PII handling are
  **required before epic #0010**, where untrusted parties submit; bounded today (only claude +
  horia, LAN-only).
