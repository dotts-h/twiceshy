---
id: 0143
title: Live model edge: a single transport blip (timeout/reset) aborts a whole multi-record prospect sweep — retry once with backoff; bump drafter timeout to match runner
status: open
severity: medium
group: 0112
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: [0112, 0140, 0142]
  regression:
assets: []
---

## Summary
0140 live attempt 7 aborted at exp-0622: the drafter's qwen call hit its 60s
HTTP client timeout awaiting headers. The record is 5.8KB and qwen answers
warm in 0.4s - a transient blip (Ollama keep-alive eviction during long
docker verify phases forces a cold reload). One blip killing a multi-record
sweep is bad run economics; a persistent outage must still abort (no silent
partials, ADR-0029/0119).

## Repro
1. Run `twiceshy prospect` over many records against a local Ollama whose
   model gets evicted mid-run (keep-alive expiry during docker verifies).
Expected: a single timed-out drafter/runner call is retried once (short
backoff); the sweep continues. Two consecutive failures abort as today.
Actual: the first transport error aborts the entire run.

## Evidence
- runs/prospect-0140-live.log: `drafter call for exp-0622: ... context
  deadline exceeded (Client.Timeout exceeded while awaiting headers)`.
- internal/agenteval/model_task_drafter.go:26 drafterHTTPTimeout=60s vs
  runner.go:22 runnerHTTPTimeout=120s (no rationale for the asymmetry).

## Acceptance
- Drafter and runner model calls retry ONCE on transport-level errors
  (timeout, connection refused/reset) with a short backoff; application
  errors (HTTP status, malformed JSON) never retry.
- drafterHTTPTimeout raised to 120s to match the runner.
- Hermetic tests: fail-once-then-succeed => success; fail-twice => error.

## Notes
