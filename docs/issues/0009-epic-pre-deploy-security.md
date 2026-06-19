---
id: 0009
title: "Epic: Pre-deploy security hardening (Tier A — single-tenant)"
status: open
severity: high
group:
depends_on: []
forgejo: 99
links:
  adr:
  prs: []
  issues: [0011, 0012, 0013, 0014]
  regression:
assets: []
---

## Summary
Security work that **gates the single-tenant deploy** (the author's own use).
Grounded in [docs/research/SECURITY_ANALYSIS.md](../research/SECURITY_ANALYSIS.md)
(Gemini research, 2026-06-18, triaged against the code). twiceshy injects record
content into downstream coding agents, so a poisoned or secret-laden record is a
stored prompt-injection / secret-leak vector — the owner's rule: **a
documented-but-quarantined hazard beats ingesting harmful content.**

## Children (build order: 0011 first — the owner's core concern)
- **#0011 ingestion safety gate** (P0) — secret scan + harmful-code heuristics +
  PII, before write, shared by the agent write-path and the importer;
  quarantine-with-flag via an additive `provenance.security_flags`.
- **#0012 injection-safe rendering** (P0) — records returned (pull) / injected
  (push) framed as data, not instructions; escaping + per-field caps. Lands the
  push half with #0002.
- **#0013 app-hardening gaps** (P1) — request body cap, query timeouts, rate
  limiting, path-under-root assertion, error-message hygiene, non-root container.
- **#0014 ongoing security maintenance** (P1) — Renovate/Dependabot, dogfood
  OSV self-monitoring of twiceshy's own deps, PR security checklist, SBOM +
  release signing.

## Already covered (do not re-do)
Timing-safe token compare, no-unauth mode, FTS5 `MATCH` escaping, query-size cap,
`govulncheck`+`gitleaks` in CI, validated-only-push invariant — see the analysis.

## Acceptance (gates the single-tenant deploy)
- [ ] #0011–#0014 closed (or P2s explicitly deferred with rationale).
- [ ] No path exists by which a secret or flagged-harmful record is silently
      ingested or promoted; such a record is quarantined + flagged.
- [ ] Returned/injected record content cannot act as instructions to the consumer.

## Notes
Multi-tenant isolation, per-tenant auth, and trial/anti-abuse are **Tier B** —
tracked in #0010 (gates the public release, not this deploy).
