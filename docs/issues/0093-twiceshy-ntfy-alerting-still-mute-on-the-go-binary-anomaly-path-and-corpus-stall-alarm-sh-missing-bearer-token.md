---
id: 0093
title: twiceshy ntfy alerting still mute on the Go binary anomaly path and corpus-stall-alarm.sh — missing Bearer token
status: closed
severity: medium
group: 
depends_on: []
forgejo:
links:
  adr:
  prs: [398]
  issues: [0038, 0072]
  regression: experience/2026 exp-2868
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

## Resolution (2026-06-28) — closed

**The code half shipped earlier, in PR #398** (`7724a7e`): `notify.New` takes a token,
`HTTPNotifier.Alert` sets `Authorization: Bearer` when non-empty, all three callers pass
`getenv("NTFY_TOKEN")`, `corpus-stall-alarm.sh notify()` gained the token, with tests
(`internal/notify/notify_test.go`, `internal/ops/scripts_test.go`). The issue was simply
left open.

**A live deployment defect remained, and was the real reason the stall alarm stayed mute.**
The Go anomaly path (promote/adapt) was fine — `validate.env` carries a topic-qualified
`TWICESHY_ALERT_URL=…/infra` + token (live POST → 200). But `/etc/twiceshy/stall-alarm.env`
still set a **bare-host** `TWICESHY_ALERT_URL=https://ntfy.radulescu.app` (no `/infra`
topic), and `corpus-stall-alarm.sh` resolves `ALERT_URL="${TWICESHY_ALERT_URL:-${NTFY_URL}}"`
— so the bare host **shadowed** the topic-qualified `NTFY_URL` the `ntfy.env` drop-in
supplies. Real signal (with the valid token): POST to the bare host → **400**; POST to
`…/infra` → **200**; POST to `…/infra` without a token → **403**. So adding the token (the
original fix) was necessary but not sufficient — a 400 (bad URL) is a different failure mode
than the 403 (no auth) this issue was filed about.

**Fixes:**
1. *Deployment* — removed the shadowing bare-host `TWICESHY_ALERT_URL` from
   `/etc/twiceshy/stall-alarm.env`; `ntfy.env` (drop-in) is now the single source of the
   topic URL + token. Verified live: the unit runs clean and the resolved URL POSTs **200**.
2. *Repo guard* — `corpus-stall-alarm.sh notify()` now warns **loudly** (stderr → journald)
   when the resolved ntfy URL has no topic path, so a "never silent again" alarm can never be
   silently mis-wired again. Guarding test: `TestCorpusStallAlarmWarnsOnTopiclessURL`.
3. *Deploy doc* — `docs/DEPLOY.md` notes ntfy URLs must be topic-qualified.
4. *Dogfood* — experience draft `exp-2868` (the bare-host-ntfy-URL → 400 trap).
