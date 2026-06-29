# ADR-0026: Runtime enforcement of experience adoption across a heterogeneous agent fleet

- **Status:** Accepted (2026-06-29) — claude drafted; horia ratified ("proceed per your
  recommendation"). Adopt **O3 (hybrid)**. Implementation follows under #0005 / the
  enforcement adapters; to reverse, supersede — do not edit.
- **Related:** [ADR-0018](ADR-0018-session-retro-capture.md) (session-retro capture spine);
  #0067 (`internal/telemetry` per-query gate-decision log, live in prod v0.2.8); #0098
  (cross-host log access so the brain's retro drain can run the #0069 join);
  #0069 / [ADR-0025](ADR-0025-session-correlation-key-on-gate-decision-telemetry.md) (the
  served→used attribution key); #0005 (the prove-or-kill trap-avoidance eval, still open);
  the "adoption is the real problem" thesis (pull-only adoption empirically fails).

## Context

twiceshy's value depends entirely on whether agents *use* it. Measured over 33h of live
traffic (2026-06-24): the **push** channel (deterministic `UserPromptSubmit` hook) fired
~2,462×, while deliberate **pull** (`search_experience`) fired ~2–5× and **feedback**
(`confirm_helpful`/`report_outcome`) fired **0×**. The pattern is structural, not a bug:

> Anything left to the model's discretion does not happen. Anything the runtime does
> *for* the model happens every time.

We run a heterogeneous fleet — Claude Code, OpenAI Codex CLI, Gemini CLI, Cursor/Composer,
and non-agentic executors calling DeepSeek/GLM/Qwen directly via OpenRouter/Ollama. MCP
itself cannot enforce consultation: the spec makes tools "model-controlled" and states the
server "cannot enforce" an interaction contract unless the host/gateway agrees to call it
(MCP 2025-06-18). So the deterministic boundary **must** live in the runner or in the
gateway that constructs the model request — never in a prompt or an MCP affordance.

Three functions, each needing a *runtime* home, not a model request:
1. **Injection** (push relevant cards) — already deterministic (push hook + `code-exec` prefetch).
2. **Retrieval** (pull on a repeated failure) — partially deterministic (error-pull `PostToolUse` hook).
3. **Feedback/usefulness** (did a served card help?) — **not** deterministic today → 0 signal.

## Decision drivers

- Must work identically across vendors whose lifecycle hooks differ (Claude Code & Gemini
  CLI expose `SessionEnd`; Codex CLI does not — only `Stop` + an outer wrapper).
- Must not depend on any model's instruction-following.
- Must keep the MCP server a knowledge service, not a policy boundary.
- Feedback must be *observed*, not *self-reported*: read what the agent did, don't ask it.

## Options

**O1 — Per-runner hooks only.** Each runner gets adapters: inject (pre), watch-errors
(post-tool), ship-transcript (session-end). *Pro:* native, lowest latency, full transcript
access. *Con:* N implementations; Codex has no `SessionEnd` (needs a wrapper); Cursor's hook
surface is unverified; non-agentic executors have no lifecycle at all.

**O2 — Central LLM gateway only.** Route every model call through one gateway (LiteLLM/
Portkey) with a pre-call hook (inject cards) and a post-call/close hook (ship exchange).
*Pro:* one enforcement point; covers any model including non-agentic; model-agnostic by
construction. *Con:* the gateway sees API turns, not the rich runner transcript; agentic
CLIs that own their own loop don't necessarily route through it; added hop/latency.

**O3 — Hybrid (recommended).** Per-runner hooks where the runner is rich and hookable
(Claude Code, Gemini CLI); the gateway as the enforcement floor for everything that calls a
model directly (non-agentic executors, and Codex's gaps via the `ask-codex`/`code-exec`
wrappers). One central "experience service" owns retrieval/ranking/feedback; thin adapters
per surface. *Pro:* best coverage, each surface uses its strongest control; no single point
forced to do everything. *Con:* two enforcement mechanisms to keep coherent.

## Decision

Adopt **O3 (hybrid)**, with the invariant: *experience lookup and feedback are properties
of the runtime, not requests to the model.* Concretely:

- **Feedback is observed post-hoc, never self-reported.** At session end the runtime ships
  the transcript; an off-pool analyzer joins it to that session's served cards (#0067 log,
  keyed by the #0069 session key) and labels each served card used / ignored / misleading.
  `confirm_helpful`/`report_outcome` remain available but are never the measurement path.
- **Per-runner session-end shippers:** Claude Code `SessionEnd` hook (LIVE 2026-06-24,
  `twiceshy-retro-ship.sh` → brain-local retro queue); Gemini CLI `SessionEnd`; Codex CLI
  `Stop` + an outer `ask-codex` wrapper `finally`; non-agentic via the orchestrator.
- **Gateway floor:** direct model calls (DeepSeek/GLM/Qwen) route through one gateway that
  injects matched cards pre-call and emits a session-end record on close — collapsing the
  non-agentic adapters into a single enforcement point (`code-exec` already centralizes
  these calls and is the natural host).
- **MCP stays the knowledge/pull interface**, never the policy boundary.
- **Forced tool calls** (`tool_choice: required`/`any`) are reserved for the narrow case of
  "this turn must emit a structured side-effect," not the default — injection is preferred.

## Consequences

- **Positive:** usefulness becomes measurable on real traffic for the first time (unblocks
  #0005); adoption no longer depends on any model's goodwill; one knowledge service, thin
  adapters; negative evidence (ignored/misleading cards) is captured as aggressively as
  positive, enabling prove-*or-kill*.
- **Negative / risks:** two enforcement mechanisms to keep coherent; auto-injected card text
  is a prompt-injection surface (MCP warns tool/served content from a corpus must be treated
  as untrusted — already mitigated by the transcript-as-DATA framing in `internal/retro`);
  hot-path hooks (`UserPromptSubmit`/pre-call inject) must stay latency-bounded (both Claude
  Code and Gemini warn synchronous hooks block the turn).
- **Follow-on work:** (1) the retro-analyzer shim (`{model,prompt,system}` → `{candidates}`,
  the contract `internal/retro` already speaks — verified 2026-06-24 against an OpenRouter/
  Ollama backend); (2) the `retro-intake` drain timer + corpus-PR wrapper (mirrors validate/
  import; ID allocation against the live corpus); (3) the served→used helpfulness analyzer
  (#0005); (4) Gemini/Codex/gateway adapters.

## Sources (research 2026-06-24, off-pool Codex)

Claude Code hooks (code.claude.com/docs/en/hooks); Codex hooks (developers.openai.com/codex/
hooks); Gemini CLI hooks (geminicli.com/docs/hooks/); LiteLLM proxy hooks (docs.litellm.ai/
docs/proxy/call_hooks); MCP 2025-06-18 (modelcontextprotocol.io/specification/2025-06-18).
