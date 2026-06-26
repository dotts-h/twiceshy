---
id: 0093
title: twiceshy ntfy alerting still mute on the Go binary anomaly path and corpus-stall-alarm.sh — missing Bearer token
status: open
severity: medium
group: 
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: [0038, 0072]
  regression:
assets: []
---

## Summary

The 2026-06-26 ntfy fix (PRs #388/#389 + `/etc/twiceshy/ntfy.env`) only covered the
**shell** notify path (`scheduled-import.sh` / `scheduled-validate.sh`). Two other ntfy
senders still omit the auth token, so against the deny-all, topic-scoped
`ntfy.radulescu.app` they 403 and the alert is silently dropped:

1. **Go binary notifier** — `internal/notify.HTTPNotifier` (the `TWICESHY_ALERT_URL`
   anomaly/guardrail path) sets `Title` + `Tags` headers but **no** `Authorization:
   Bearer` header.
2. **`scripts/corpus-stall-alarm.sh`** — has its own `notify()` that also omits the token
   (was not touched by PR #389).

## Repro
1. `curl -s -o /dev/null -w '%{http_code}' -d x https://ntfy.radulescu.app/infra` → **403**
   (no auth); add `-H "Authorization: Bearer $NTFY_BOT_TOKEN"` → **200**.
2. Configure `TWICESHY_ALERT_URL=https://ntfy.radulescu.app/infra` and trip a guardrail
   anomaly (or call `HTTPNotifier.Alert`).
Expected: a notification arrives in the `infra` topic.
Actual: `HTTPNotifier` logs `alert post returned non-2xx status=403`; nothing delivered.
Same for a stall-alarm trigger.

## Evidence
- `internal/notify/notify.go` `Alert()` sets only `Title`/`Tags`, never `Authorization`.
- `ntfy.radulescu.app` is deny-all + topic-scoped: no topic → 400, no token → 403
  (verified live 2026-06-26; bot token writes to `infra` → 200).

## Notes

Fix mirrors PR #389:
- **(1)** `notify.New()` / `HTTPNotifier` read a token (e.g. `NTFY_TOKEN` env) and set
  `Authorization: Bearer <token>` when non-empty; add a unit test asserting the header is
  present on the request (and absent when the token is empty). Wire `NTFY_TOKEN` into the
  serving/validate process env (the same `/etc/twiceshy/ntfy.env` already exists).
- **(2)** `corpus-stall-alarm.sh` `notify()` gets the same `${NTFY_TOKEN:+-H "Authorization:
  Bearer $NTFY_TOKEN"}` treatment; its unit (`twiceshy-stall-alarm.service`) references
  `/etc/twiceshy/ntfy.env`.

Context: real topic is `infra`; bot token in `~/.config/brain/secrets.env`
(`NTFY_BOT_TOKEN`). Related: #0038 (route guardrail trips → ntfy notify seam), #0072
(corpus pipeline hardening / ntfy-on-failure + stall-alarm). The shell path was fixed in
PRs #388 (depletion alert) and #389 (Bearer token) on 2026-06-26.
