---
id: 0099
title: Usage-judge 502: retro analyzer shim rejects the verdicts schema, zeroing the #0069 confirmed-helpful signal
status: closed
severity: high
group: 0064
depends_on: []
forgejo:
links:
  adr: docs/adr/ADR-0018-session-retro-capture.md
  prs: [428]
  issues: [0098, 0069, 0067]
  regression: docs/REGRESSIONS.md#usage-judge-502-0099
assets: []
---

## Summary

The retro **helpfulness join** (#0069) now matches sessions (the #0098 salt fix
unblocked it) but logs `confirmed 0 helpful` on every session, because the
**usage judge 502s**. Root cause: the join's `ModelUsageJudge` and the
draft-extraction `ModelAnalyzer` are wired to the **same `:8729` analyzer shim**
(`TWICESHY_RETRO_URL`), but the shim hard-validates the *extraction* contract and
rejects the *usage* response. The salt fix exposed this — the served→used metric,
the entire point of the measurement chain, still reads 0.

## Repro
1. With the salt fix deployed (#0098 / PR #425), run the scheduled retro drain
   (`scripts/scheduled-retro.sh`) over a transcript whose session was served cards.
2. Observe the join step in `journalctl -u twiceshy-retro`:

Expected: the usage judge returns `{"verdicts":[{"id":"exp-NNNN","used":…}]}`,
the join attributes used cards, and the drain logs `confirmed N helpful` (N≥0,
non-error).

Actual:
```
WARN retro helpfulness join: usage judge failed session=git-twiceshy-… \
  error="retro: usage status 502: {\"error\": \"analyzer returned no candidates array\"}"
  … confirmed 0 helpful
```

## Evidence
- Live drain (2026-06-29 08:18 run, PR #71): every session logged the 502 above
  then `confirmed 0 helpful`.
- Shim rejection — `~/work/twiceshy-retro-analyzer/retro_analyzer_shim.py:215`:
  ```python
  if not isinstance(result, dict) or not isinstance(result.get("candidates"), list):
      return self._json(502, {"error": "analyzer returned no candidates array"})
  ```
- Usage judge contract — `internal/retro/model_usage.go` (`wireVerdicts`):
  parses/requires `{"verdicts":[{"id","used"}]}`, no `candidates` key.
- Drain wiring — `cmd/twiceshy/retro.go:67` builds `NewModelUsageJudge(cfg)` from
  the same analyzer config (endpoint `TWICESHY_RETRO_URL` = `:8729`, `retro.env`),
  so both retro roles hit the one extraction-only shim.

## Notes
- **Fix (chosen, #0098 follow-on):** make the shim schema-flexible — pass the
  model's strict-JSON object through when it carries a `candidates` list **OR** a
  `verdicts` list; keep the 502 fail-safe only when *neither* is present (so a
  genuinely broken/empty model response still leaves the transcript queued). The
  drain already points both roles at one URL, so accept-either matches the wiring
  without touching the Go side.
- **Untracked-artifact gap (fixed here):** the retro analyzer shim lives only in
  `~/work/`, untracked — unlike its siblings `scripts/{gemini,sonnet}-judge-shim.py`.
  This issue brings it into `scripts/retro-analyzer-shim.py` and adds a guarding
  contract test (`scripts/retro-analyzer-shim.test.sh`, run by `make ci` via
  `test-scripts`), then deploys from the committed copy (exp-2840: stale binary
  silently breaks a scheduled drain).
- Continues #0098 (the salt coherence fix that surfaced this) under epic 0064.

## Resolution (closed 2026-06-29, PR #428)

Shim now passes the model's strict-JSON through when it carries a `candidates`
**or** `verdicts` list; 502 fail-safe only when neither is present. The
previously-untracked shim is now `scripts/retro-analyzer-shim.py` under the
hermetic contract test `scripts/retro-analyzer-shim.test.sh` (run by `make ci`).

Verified on real signal — the deployed `:8729` shim against the live `gpt-oss:20b`
now returns `HTTP 200 {"verdicts":[{"id":"exp-0149","used":true}]}` (was `502 "no
candidates array"`). Dogfood record exp-3222 (corpus PR #73); regression row in
`docs/REGRESSIONS.md`.
