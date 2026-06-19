---
id: 0019
title: Extend ingestion screen to repro-script content + execution trust boundary
status: closed
severity: high
group: 0015
depends_on: []
forgejo: 109
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

Must-have C (ADR-0011 §4): extend `internal/screen` to screen **repro-script
content** (the executable text), not just record prose — and enforce the **trust
boundary**: only PR-reviewed + ingest-screened repros may be executed by the
broker. A repro legitimately contains code, so calibrate rules for *scripts*
(don't false-positive on every test) while still catching exfil / secret /
destructive payloads.

## Touches

`internal/screen/screen.go` (`Scan(texts...)`), wired into ingest + the broker gate.

## Acceptance

- [x] Screen accepts repro-script content; flags secrets / exfil / destructive ops
- [x] Broker refuses to run an unscreened / flagged repro (trust boundary enforced)
- [x] Test-first; calibrated against existing repro fixtures (no false reject)

## Resolution (done 2026-06-18)

- `internal/screen` gained `ExecutionHazards(findings)` — the calibrated gate for
  *script* content: it keeps `secret` + `harmful-code` findings (the things that
  make a script unsafe to run) and drops `pii` (a test fixture may legitimately
  contain an email/IP, and the execute phase has no network to exfiltrate). The
  existing harmful-code rules are already exec-only (pipe-to-shell, reverse shell,
  `/dev/tcp`, fork bomb, root `rm`), so they fit scripts without false-positiving
  on ordinary test code.
- **Trust boundary enforced at the broker** (`internal/repro`): `Run` screens
  every staged file's CONTENT via `ExecutionHazards` and refuses *before any
  docker work* if a hazard is present. The broker is the right chokepoint — it is
  the only place the script content is guaranteed present and the only place it
  can execute, so even a buggy/compromised caller can't run flagged code. (Ingest
  `Prepare` was the wrong home: its `repo` arg is a module identifier not an FS
  path, and propose-only drafts carry repro *paths*, not content — the file is
  added later in the PR.)
- **Loopback false-positive fixed:** `pii:private-ip` no longer matches
  `127.0.0.0/8`. Loopback is localhost, not PII, and Docker's embedded DNS at
  `127.0.0.11` (exp-0016) appears legitimately in sandbox/repro scripts.
- Tests: harmful script refused, embedded-secret refused, benign script with
  loopback+email allowed, and a calibration test that screens the repo's real
  repro fixtures and asserts no false reject. Verified under real runsc too.
