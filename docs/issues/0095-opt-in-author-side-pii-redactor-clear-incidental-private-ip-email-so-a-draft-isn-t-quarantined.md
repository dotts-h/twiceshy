---
id: 0095
title: Opt-in author-side PII redactor — clear incidental private-IP/email so a draft isn't quarantined
status: in-progress
severity: low
group:
depends_on: []
forgejo: 406
links:
  adr: ADR-0011
  prs: [405]
  issues: [0090, 0091]
  regression:
assets: []
---

## Summary

The ingestion safety gate (`internal/screen`, #0011) is **detect → flag → quarantine for human
review** by design — it deliberately never auto-mutates a record (the trust boundary is human/judge
review on the PR; silently rewriting a recorded claim would corrupt the corpus). That is correct for
**secrets** (a leaked secret must be *rotated*, not masked-and-kept) and for anything load-bearing.

But for **incidental, low-severity PII** — an RFC-1918 IP or an email that wandered into an
agent-authored note — the only escape today is a hand-edit + re-propose round-trip. (Live example:
`record_experience` quarantined a draft solely because a `guard.guarding_test` curl line contained a
LAN IP `192.168.50.x`; the lesson was generic and the IP was noise.) We want to remove that friction
**without** weakening the trust model: an **opt-in, author-side** redactor the author invokes
deliberately, not a silent ingest-path mutation.

## Repro
1. Author a draft whose only hazard is a private IP or email (no secret), e.g. a `guarding_test`
   containing `http://10.0.0.5:8722/`.
2. `record_experience` / `ingest.Prepare` runs `screen.Scan` → `security_flags: [pii:private-ip]`,
   record forced `quarantined`, promotion blocked.

Expected: an opt-in tool that returns the draft with the incidental PII replaced by stable
placeholders (so re-screening finds no PII and the record is promotable), the author reviewing the
result before using it.
Actual: no such helper — the author must hand-redact and re-propose.

## Evidence

- `internal/screen/screen.go` — `Scan`/`Flags`/`mask`; `mask()` is leak-safety for a *Finding*, not
  a sanitized record. PII regexes: `private-ip` (`10.x.x.x` / `192.168.x.x`), `email`.
- `internal/ingest/prepare.go` — policy is quarantine-with-flag (or `RejectOnFlag`); never mutates.
- `cmd/twiceshy/screen.go` — the existing `twiceshy screen` CLI (stdin → flags; non-zero on secret).
- Live trigger: corpus record `exp-2814` (hand-redacted `192.168.50.244` → `<MCP_HOST>` to clear the
  flag for promotion).

## Notes

**Design — keep the trust model, add an opt-in escape only for low-severity PII.**

Phase 1 (this issue / first PR):
- **`screen.Redact(text string) (redacted string, redactions []Finding)`** — a pure, deterministic
  function that replaces **PII matches only** (`private-ip`, `email`) with stable placeholders
  (`<REDACTED-IP>`, `<REDACTED-EMAIL>`), reusing the *same* `patternRules` regexes as `Scan` (single
  source of truth), and returns what it changed. **Secrets and harmful-code are never redacted** — a
  secret must be rotated, and masking it would fake "handled". Replaces every occurrence.
- **`twiceshy screen -redact`** — opt-in flag on the existing CLI: print the redacted text to stdout
  (pipeable) and a summary of redactions to stderr. A **secret still hard-fails** even with `-redact`
  (exit non-zero, text NOT emitted) — you cannot redact your way past a secret. Default (no flag) =
  today's behavior, unchanged.

Invariants (the gate — property tests):
1. **Round-trip:** `screen.Scan(Redact(t)) ` has **no `pii:*` finding** — redaction clears the flag.
2. **Secrets untouched:** a secret in `t` survives `Redact` and `Scan` still flags it.
3. **Deterministic & pure:** `Redact(t)` twice → identical output; non-matching text unchanged.
4. **No leak:** the redacted text contains neither the original IP nor the original email.

Phase 2 (follow-up, NOT this PR — noted so we don't build ahead): an opt-in `redact_pii` parameter on
the `record_experience` MCP tool that, when the *only* findings are low-severity PII, returns a
redacted draft suggestion (still quarantined) — closing the loop at the exact friction point. Kept
separate because it touches the MCP tool schema + server handler.

Relation: complements #0090 (authored-record similarity) and #0091 (`twiceshy author` scaffold) as
author-time hygiene; honors ADR-0011's gate (detector stays pure; policy/opt-in lives in the caller).
