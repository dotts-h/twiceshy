---
id: 0086
title: Hybrid advisory-panel frontier judge — Gemini primary, Sonnet fallback on quota exhaustion
status: closed
severity: medium
group: 0034
depends_on: []
forgejo: 459
links:
  adr: docs/adr/ADR-0016-advisory-class-panel-promotion.md
  prs: []
  issues: [0016, 0084]
  regression:
assets: []
---

## Summary
The advisory panel's frontier (second-family) seat ran on `claude-sonnet-4-6`,
spending the Anthropic weekly pool on every advisory promotion of an autonomous
loop. ADR-0016 §5 already sanctions **Gemini** for advisory records (they are public
OSV/GHSA data), and a Gemini judge shim is deployed (`:8724`, `gemini-2.5-flash`).

But a straight swap to Gemini is unsafe for an unattended ~48-runs/day loop: the
shim is **free-tier** (~1500 req/day). The loop would exhaust the daily quota part-way
through each day → every panel call 429s → the fail-safe panel keeps every record
quarantined → **promotions stall daily**. (The shim already paces RPM + retries 429,
but a *daily*-cap exhaustion can't be retried away.)

## Fix
A `judge.FallbackJudge` (new) wraps the frontier seat: **primary** = Gemini,
**secondary** = Sonnet, consulted **only on a primary error** (endpoint down /
throttled / quota exhausted). A primary **reject** does NOT fall back — it is a real
verdict, not a failure (falling back would double-spend and let the secondary
override a legitimate decline). If both error, the primary error is surfaced so the
record stays quarantined (fail-safe). Net: off-pool on the happy path, Sonnet covers
quota exhaustion, no stall.

Wired in `cmd/twiceshy` behind `TWICESHY_PANEL_JUDGE_FALLBACK_URL` /
`TWICESHY_PANEL_JUDGE_FALLBACK_MODEL`; unset = today's single-judge behaviour. Panel
family-diversity holds (gpt-oss + gemini are distinct families; on fallback, sonnet
is also distinct from gpt-oss); the runtime `verdict.Model` records whichever model
answered, so the manifest stays honest.

## Deployment (post-merge, in order — do NOT reorder)
1. Merge this PR; build + install the new prebuilt engine (`TWICESHY_BIN`).
2. **Only then** set in `validate.env`:
   - `TWICESHY_PANEL_JUDGE_URL=http://localhost:8724`
   - `TWICESHY_PANEL_JUDGE_MODEL=gemini-2.5-flash`
   - `TWICESHY_PANEL_JUDGE_FALLBACK_URL=http://localhost:8725`
   - `TWICESHY_PANEL_JUDGE_FALLBACK_MODEL=claude-sonnet-4-6`

   Setting the Gemini primary against the OLD binary (which ignores `*_FALLBACK_*`)
   would make Gemini the SOLE judge → the daily-stall this issue prevents.
3. Watch the first day's manifests: `verdict.Model` should be mostly
   `gemini-2.5-flash`, with `claude-sonnet-4-6` appearing once the daily quota trips.

## Acceptance
- [x] Primary success → primary verdict; secondary untouched.
- [x] Primary error → secondary verdict (no stall).
- [x] Primary reject → stands; no fallback.
- [x] Both error → primary error surfaced (fail-safe quarantine).
      — `internal/judge/fallback_test.go`.
- [ ] Deployed: first-day manifests show the Gemini→Sonnet fallback firing on quota.
