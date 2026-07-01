---
id: 0116
title: Metrics digest: daily push/join/promote health digest to ntfy
status: closed
severity: medium
group: 
depends_on: []
forgejo: 495
links:
  adr:
  prs: [486]
  issues: [0106, 0067, 0069]
  regression:
assets: []
---

## Summary
A daily digest (script + systemd timer on the brain, alongside the existing
`twiceshy-audit`/`twiceshy-validate`/`twiceshy-corpus-sync` timers) posting to
ntfy: last-24h push served-rate vs the 70% pre-fix baseline (ADR-0028/#0106),
per-channel counts, top served ids with `query_text` samples (opt-in,
`internal/telemetry/decision.go:39`'s `Decision.QueryText`, #0109), usage sums
(`pushed`/`retrieved`/`confirmed_helpful`), retro join outcomes ("confirmed N
helpful" plus unprocessable counts from `journalctl -u twiceshy-retro`), and
promote/hold counts from the validate runs' `RunManifest`
(`internal/promote/manifest.go:34`, `Counts`/`Actions[].Outcome`). Closes epic
#0106's last open acceptance box ("post-deploy telemetry shows served-rate on
prompt-triggered push well below the 70% baseline") with data instead of a
one-off pull. Includes a small engine change: telemetry `Decision`
(`internal/telemetry/decision.go:39`) currently has no `trigger` field —
`Channel` distinguishes `"push"`/`"search"` but not `trigger ∈ {"", "prompt",
"error"}` (`PushArgs.Trigger`, `internal/server/push.go:38`) — so
prompt-vs-error served-rate cannot be split from the decision log as it
stands. This issue adds `Decision.Trigger`, which requires a serve redeploy
before the field appears in new decisions.

## Repro
1. Query the live gate-decision log for prompt-triggered vs error-triggered
   served-rate.
Expected: two distinct served-rate numbers, one per trigger.
Actual: `Decision` only carries `Channel` (`"push"`/`"search"`), not the
`trigger` value the push handler already receives — the two triggers are
indistinguishable in the log today.

## Evidence
- `internal/telemetry/decision.go:39` (`Decision`) has no `Trigger` field;
  `internal/server/push.go:21` (`PushArgs`) and `:38` (`Trigger`) is where the
  trigger value already exists on the request but is not threaded into the
  recorded `Decision`.
- `internal/promote/manifest.go:34` (`RunManifest`) and `:17`–`28`
  (`RecordAction.Outcome`, one of `promoted|held|ineligible|demoted|disputed|
  orphan`) is the existing machine-readable source for promote/hold counts —
  no new plumbing needed there, only a reader.
- Epic #0106's acceptance checklist has an open box for exactly this
  post-deploy measurement.
- `scripts/daily-audit.sh` is the existing sibling script (ntfy digest of the
  latest promote manifest) this digest's shape should follow.

## Acceptance
- `Decision.Trigger` is added (`internal/telemetry/decision.go:39`), populated
  by the push handler from `PushArgs.Trigger`; a redeploy note is included
  since existing running instances won't emit it until restarted.
- The digest script computes and posts: served-rate by trigger vs the 70%
  baseline, per-channel counts, top-N served ids with `query_text` samples
  when telemetry query-text capture is on, usage sums, retro join outcomes
  (parsed from `journalctl -u twiceshy-retro`), and promote/hold counts from
  the latest `RunManifest`.
- A systemd `.service`+`.timer` pair is added under `scripts/`, matching the
  existing `twiceshy-*` unit naming and env-knob conventions.
- Epic #0106's "post-deploy telemetry" acceptance box can be checked off from
  this digest's output.

## Notes
This is reporting/observability only — it changes no gate behavior. The
`Decision.Trigger` addition is the one non-trivial engine change bundled in;
everything else reads existing artifacts (`RunManifest`, the decision log,
usage counters, retro journal).
