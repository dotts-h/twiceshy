# ADR-0030: Public alpha as a hosted multi-tenant remote-MCP service (open-core)

- **Status:** Proposed (deciders: claude drafted from the 2026-07-06 productization
  discussion; horia to ratify).
- **Related:** ADR-0002 (AGPL-3.0 + CLA — the licensing posture this decision
  activates commercially), ADR-0001 §6 (PR/quarantine trust boundary — reused as
  the contribution moderation pipeline), ADR-0013/0016/0020 (the judge pipeline
  that makes untrusted contributions admissible at all), #0088 (corpus coverage)
  and #0094 (capture tooling — the contribution flywheel's client side), #0097
  (the exp-0003 lesson: remote MCP speaks streamable HTTP — the transport the
  server already implements).

## Context

twiceshy's long-term value is the **corpus**: validated, deduplicated engineering
traps served to agents at decision time. Growing it past one operator's sessions
requires other people's failures — a contribution flywheel — and the eventual
business is open-core: AGPL self-hosting stays free (ADR-0002 anticipated this),
while a hosted, token-gated service and private team corpuses are the paid tier.

Delivery needs no client app: the server already exposes the pull channel as MCP
over **streamable HTTP** behind bearer auth (`internal/server/server.go`), and
every major coding agent (Claude Code, Cursor, Windsurf, Copilot, Codex CLI) is a
remote-MCP client. Onboarding is one config entry with a token. The push channel
(prompt-time card injection) requires a client-side hook and is explicitly out of
the alpha's required scope — pull-only is the alpha.

What is missing is the production layer around the existing single-tenant server:
per-user tokens, quotas, per-tenant telemetry, a public deployment off the
homelab, a landing page for signup/product explanation, and contribution terms.
The write path (`record_experience`, `report_outcome`) is the flywheel but also
the poisoning surface; the quarantine→judge→soak pipeline (ADR-0013) is the
moderation system, extended with per-origin trust tiers (#0118 already opened
that door for the importer class).

## Decision

Ship a **public alpha as one hosted multi-tenant instance** of the existing Go
binary, delivered exclusively as a **remote MCP endpoint** (streamable HTTP +
per-user bearer tokens). Scope, in phases gated by the epic's children:

1. **Read alpha (launch):** `search_experience`/`get_experience` for token
   holders, per-token quotas + rate limits, per-tenant usage telemetry, landing
   page with self-serve token signup and product explanation, deployment off the
   homelab (isolated VPS/VM + TLS; NOT the NAS or the brain).
2. **Write alpha (fast follow, same epic):** `record_experience`/`report_outcome`
   opened per-token behind a low-trust origin tier: hostile-input PII/secret
   scrubbing hardened for untrusted authors, contributions land quarantined and
   flow through the unchanged judge/soak pipeline, contribution data-license
   terms accepted at signup.
3. **Out of alpha scope:** push-channel hosting, private team corpuses, billing,
   OAuth (plain bearer tokens suffice; MCP-spec OAuth can come later).

The corpus ships with the service (the hosted instance serves the public
validated set); a curated snapshot may also be published with the AGPL code, but
the hosted corpus + tokens are the controllable surface.

## Options considered

- **Hosted multi-tenant MCP (chosen):** zero client install, every agent already
  speaks it, tokens are the control point, single binary + SQLite scales fine
  for an alpha.
- **Self-host-only open-sourcing:** zero ops and zero data risk, but no
  contribution flywheel and no controllable tokens — it is the fallback posture,
  not the goal; it remains available regardless (AGPL).
- **Client app / CLI distribution:** highest friction, duplicates what MCP
  clients already do; rejected for the alpha (capture tooling à la #0094 comes
  later as an optional enhancer, not a prerequisite).

## Consequences

- The single shared bearer token becomes a **token table** (issue/revoke/quota/
  rate-limit, per-tenant usage attribution) — the main engine work.
- Telemetry gains a tenant dimension; the operator dashboard and the daily digest
  read from it. The #0122 liveness lesson applies: alarm on throughput-zero and
  hostile-input floods, not just errors.
- A public write path makes the judge pipeline security-relevant: its failure
  modes (exp-4454, #0123) must alert loudly, and per-origin trust tiers decide
  what an anonymous token's records may ever reach (never the push channel —
  ADR-0001 §4's quarantine invariant already guarantees the floor).
- The homelab is not the blast radius: the public instance runs isolated, with
  its own corpus clone and secrets; the LAN instance stays private.
- Cold-start honesty: launch value = the public validated corpus; the measurement
  work (#0067/#0069 chain) stays priority one, because the flywheel only spins if
  serving demonstrably helps.
