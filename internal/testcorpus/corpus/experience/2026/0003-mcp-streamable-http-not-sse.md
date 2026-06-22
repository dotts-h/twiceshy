---
schema_version: 1
id: exp-0003
kind: convention
status: validated
title: "Remote MCP servers speak streamable HTTP — the standalone HTTP+SSE transport is deprecated"

applies_to:
  - ecosystem: "MCP"
    package: "modelcontextprotocol/specification"
    versions: { introduced: "2024-11-05", fixed: "2025-03-26" }

resolution:
  root_cause: >
    The original remote transport (protocol revision 2024-11-05) used a
    two-endpoint HTTP+SSE pair: a GET-only SSE stream plus a separate
    message-POST endpoint. Revision 2025-03-26 replaced it with "Streamable
    HTTP": one endpoint that accepts POSTed JSON-RPC and may stream the
    response (SSE is now an internal detail of that single endpoint, not a
    transport). Tutorials, older SDK examples and LLM training data still
    show the deprecated pair.
  fix: >
    Serve a single MCP endpoint speaking streamable HTTP and connect clients
    to it (Claude Code: `claude mcp add --transport http <name> <url>`).
    Build on an SDK's streamable-HTTP handler; do not hand-roll an /sse +
    /messages pair.
  dead_ends:
    - tried: "exposing the classic /sse + /messages endpoint pair"
      why_it_failed: >
        Deprecated since protocol 2025-03-26; current clients negotiate
        streamable HTTP and never complete the legacy handshake, failing
        with opaque connection errors. Backwards-compatibility shims exist
        in some SDKs but are a dead weight for a new server.

guard:
  repro: null
  guarding_test: "TestServerSpeaksStreamableHTTP"

provenance:
  source: { author: "horia", session: null, pr: null }
  recorded_at: 2026-06-12
  validated_at: 2026-06-12
  valid: { from: 2026-06-12, until: null }
  superseded_by: null
  usage: { retrieved: 0, confirmed_helpful: 0, last_hit: null }
---

## The convention

Any remote MCP server built here uses the **streamable HTTP** transport:
one endpoint, JSON-RPC over POST, optional SSE-framed streaming *within*
that response. The standalone HTTP+SSE transport (separate GET stream +
POST endpoint, protocol revision 2024-11-05) is deprecated and must not be
built, even when a tutorial, an older SDK example, or the model's training
data suggests it — this is a textbook case of agents reproducing stale
context (research §1: stale context in, stale code out).

This record has no `symptom` block: nothing errors when you *write* the
wrong transport; clients just fail to connect later. It is a convention
card — the distilled "do this, not that" — rather than an episodic trap.

## Practical notes

- stdio remains the right transport for local single-process servers; this
  convention is about remote/network servers.
- Authentication for our deployment is bearer-token middleware in front of
  the MCP handler (ADR-0001 §9); Claude Code passes
  `--header "Authorization: Bearer …"`.
- twiceshy's own pull channel (`search_experience`, `get_experience`) is
  the reference implementation; its guarding test drives a real MCP
  client handshake over streamable HTTP against the running handler.
