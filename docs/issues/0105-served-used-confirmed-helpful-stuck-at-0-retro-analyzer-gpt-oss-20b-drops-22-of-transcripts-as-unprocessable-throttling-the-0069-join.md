---
id: 0105
title: Served→used confirmed-helpful stuck at 0: retro analyzer (gpt-oss:20b) drops ~22% of transcripts as unprocessable, throttling the #0069 join
status: open
severity: high
group: 0064
depends_on: []
forgejo: 437
links:
  adr: docs/adr/ADR-0018-session-retro-capture.md
  prs: []
  issues: [0005, 0069, 0099, 0100]
  regression:
assets: []
---

## Summary

The #0069 served→used helpfulness signal — the core "are agents actually *helped*" metric and the
input to the #0005 prove-or-kill eval — **has never gone positive: confirmed-helpful = 0 across all
38 drain cycles.** The root cause is **not** primarily "cards aren't used." It is that the **retro
analyzer (`gpt-oss:20b` via the `:8729` shim) is too unreliable to even PROCESS most served
sessions**: on real transcripts it frequently returns **empty** or **malformed (truncated) JSON**, so
the transcript is dropped as "unprocessable" *before it ever reaches the usage-judge/join*. The
served→used chain (now plumbed correctly post #0099/#0100) is therefore throttled at the **analysis**
stage, not the join. Measured chronic failure ≈ **22%** (66 unprocessable vs 228 drafts ever created);
**3 of 5** served sessions failed analysis in a targeted run.

## Repro
1. Isolate the queued `claude-code` transcripts whose sessions were served cards (hash `session_id`
   with the telemetry salt = `TWICESHY_TOKEN` fallback, intersect with the `:8722` gate-decision log's
   served set).
2. Run `twiceshy retro-intake` **non-dry-run** (so the #0069 join fires) on those served sessions
   against the corpus + the real `TWICESHY_TELEMETRY_LOG`.

Expected: each served session is analyzed, the usage-judge assesses the served cards, and
confirmed-helpful reflects real usage.

Actual: **3 of 5** served sessions dropped as `unprocessable after 3 attempts` — retro analyzer
`status 502`: `"empty model content"` (×2) and `"Expecting ',' delimiter: line 1 column 4764"`
(malformed/truncated JSON, ×1). The **1** that processed → **`confirmed 0 helpful`** (n=1,
inconclusive on actual helpfulness).

## Evidence
- **Drain history: 66** `unprocessable`/`empty model content`/`status 502` events vs **228** `created
  exp-` drafts (≈22% analysis-failure rate). **All 38** drain cycles log `confirmed 0 helpful`; never
  positive.
- Of **11** already-joined `claude-code` sessions, only **2** were actually served cards (the rest had
  nothing to confirm); both `confirmed 0`. Plus **5** served sessions still queued, undrained.
- Analyzer shim is **healthy** (`{"ok":true,"model":"gpt-oss:20b","role":"retro-analyzer"}`) and
  returns valid JSON on a trivial prompt (`{"candidates":[]}`) — the failures are on **real (7–18 KB)
  transcripts**: empty content, or JSON truncated mid-object.
- Telemetry plumbing **verified correct** (so the join *can* match, it just rarely gets a processed
  transcript): the push hook forwards `session_id` (**358/417** push decisions carry a session), the
  `SessionEnd` shipper stamps the same raw id, and the salt/hash math reproduces real session↔served
  pairings (**47** distinct served sessions).

## Notes
- **Distinct from #0099 and #0100.** #0099 = the shim hard-validated only `{candidates}` so the
  usage-judge 502'd (schema bug, fixed). #0100 = dead-letter on first failure, no retry (fixed). **This
  is the MODEL itself** (`gpt-oss:20b`) emitting empty/malformed output on real transcripts — the shim
  correctly rejects it, the retries exhaust, and the transcript is skipped.
- **Gates #0005 (prove-or-kill):** served→used cannot be measured while ~most served sessions are
  unprocessable. The fleet adapters (epic #0101, just completed) now feed traffic, but the analyzer is
  the bottleneck.
- **Fixes to evaluate:** (a) a more JSON-reliable / larger analyzer model on the Ollama VM (RTX 4080
  SUPER has headroom), or constrained/grammar/JSON-mode output; (b) JSON-repair / salvage of truncated
  output before rejecting; (c) bound or chunk the transcript fed to the model (the truncation hint
  `char 4763` points at output-length glitches); (d) raise the retry budget and **alert on a chronic
  per-model failure rate** so this can't silently sit at 0.
- Connected: #0069 (the join), #0067 (decision log), #0005 (the eval it gates), #0099/#0100 (prior
  served→used fixes), ADR-0018 (retro capture). Surfaced during the epic-0101 status review 2026-06-29.

## Progress (2026-06-30)

The alerting half of item (d) is done: `runRetroIntake` now wires a `notify.Alerter`
(`TWICESHY_ALERT_URL`/`NTFY_TOKEN`, same convention as promote/adapt), and `drainRetro`
fires a one-shot `retro-analyzer-unreliable` alert when a drain's unprocessable rate
exceeds 20% over a minimum sample of 5 attempted transcripts — so this can no longer
silently sit at 0 for weeks before a human-initiated audit notices (exactly how this
issue itself was surfaced). Guarded by `TestDrainRetro_ChronicFailureRate_Alerts` /
`_NoAlertOnSuccess` / `_NoAlertBelowMinSample` (`cmd/twiceshy/retro_test.go`).

**Still open:** the retry-budget half of (d), and fixes (a)/(b)/(c) — the analyzer
itself still drops ~22% of real transcripts. The alert makes the failure visible; it
does not reduce it.
