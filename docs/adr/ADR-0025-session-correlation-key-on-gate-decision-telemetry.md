# ADR-0025: A salted session-correlation key on the gate-decision telemetry

- **Status:** Accepted (2026-06-23) — claude proposed; horia ratifies. Enables the
  authoritative attribution half of the session-retro helpfulness signal (#0069).
- **Related:** [ADR-0018](ADR-0018-session-retro-capture.md) (the retro capture spine
  whose transcript this join completes); #0067 (the per-query gate-decision telemetry
  this extends, `internal/telemetry`); #0069 (the helpfulness signal this unblocks);
  [ADR-0013 §4](ADR-0013-closed-loop-autonomous-validation.md) (the usage signal is
  off the hot path and never influences ranking).

## Context

#0069 closes the measurement loop: *did a served card actually help?* The retro analyzer
holds a session's transcript (with its raw MCP session id) and judges, per served card,
used-vs-ignored. To record only **real** signals it must confirm a verdict's card was
actually **served in that session** — not trust a prompt-injectable model's id blindly
(exactly the trust-boundary gap the #0069 slice-1 review flagged). The #0067 decision log
records, per query, which cards were served — but carries **no session key**, so a
transcript cannot be joined to its served set.

The telemetry log is deliberately privacy-preserving: it persists a *salted hash* of the
query, never the raw text (#0067). A session correlation key must hold that line.

## Decision

Add a **salted session hash** to `telemetry.Decision` (`session`, omitempty), stamped on
the **search** (pull) channel from the MCP session id (`req.GetSession().ID()`), hashed
with the same per-deployment salt the query already uses (`Recorder.Hash`). The raw
session id is **never persisted** — only its hash, exactly like `query_hash`. A request
with no session records no key (`""`) and is therefore attributable to no session.

A reader — `telemetry.ServedInSession(path, sessionHash)` — returns the union of served
ids for a session across the active log and its one rotated generation. The retro join
(follow-up) hashes the transcript's session id with the deployment salt, calls it to
obtain the **served set**, then confirms only verdicts that are Used **and** in that set.

**Scope: pull channel only.** `/push` is a hook POST, not an MCP session, so it carries
no session linkage; the deliberate-retrieval (search) path is where an agent pulls a card
within a session, and the signal that matters.

## Consequences

- **#0069's authoritative attribution is unblocked** without trusting model-supplied ids:
  the served-set cross-check now has a real source. The join orchestration, the off-pool
  usage judge, and the precision/recall reporter are the tracked follow-up.
- **Privacy line held.** Only a salted hash is stored (no raw session id), consistent with
  `query_hash`; the residual is the same single-tenant exposure ADR-0018 already accepts.
  Multi-tenant (epic #0010) inherits the ADR-0018 PII/threat-model flag.
- **No ranking impact.** The key lives in the write-only decision log, structurally
  separate from the index (ADR-0013 §4); it cannot influence retrieval.
- **Push attribution deferred.** A hook-injected card has no clean session link here; if
  ever needed, the hook would have to supply a correlatable id (out of scope).
