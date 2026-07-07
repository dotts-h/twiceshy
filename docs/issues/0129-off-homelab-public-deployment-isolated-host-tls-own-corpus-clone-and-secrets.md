---
id: 0129
title: Off-homelab public deployment: isolated host, TLS, own corpus clone and secrets
status: closed
severity: medium
group: 0124
depends_on: []
forgejo: 530
links:
  adr: ADR-0030
  prs: [517, 537]
  issues: [0122, 0131]
  regression:
assets: []
---

## Summary

Stand up the public instance off the homelab: an isolated VPS/VM (NOT the NAS,
NOT the brain) running the same container image, its own corpus clone and
secrets, TLS (either native or via a tunnel/proxy in front), backups, and the
watchdog/alarm set (disk, liveness #0122). The LAN instance stays private and
unchanged; deploy is compose + the existing systemd timer patterns.

## Notes

Blast-radius rule from ADR-0030: nothing on the public host may reach the LAN.
One-way corpus publishing (LAN validated set → public instance) is the only
link, via the existing git-based corpus sync pattern.

## Status 2026-07-06

Deploy artifacts and runbook shipped in PR #517 (`deploy/public-alpha/`,
`docs/DEPLOY-public-alpha.md`). BLOCKED on two operator decisions — hosting
provider and domain — see the runbook's OPEN DECISIONS section. Actual
stand-up of the public host cannot proceed until those are made.

## Status 2026-07-07 — CLOSED, live

Decisions made (Hetzner CX23 `twiceshy-alpha` @ fsn1; domain `twiceshy.app`
via Cloudflare Registrar — the runbook's `twiceshy.dev` placeholder turned out
to be someone else's registered domain) and the instance brought up same day.
All five runbook smoke tests pass over public TLS: healthz/readyz (4351
records), signup roundtrip, MCP initialize, `search_experience` returns cards,
bad token 401; landing `/`, `/docs`, `/terms` serve. Corpus-refresh timer
(30 min), disk alarm, nightly backup, and scoped ntfy alerting live.

One deliberate deviation from the shipped design, on operator decision: **both
git repos stay private** (the corpus is the monetizable surface per ADR-0030) —
corpus pulls use a dedicated read-only `corpus-pull` Forgejo account scoped to
the corpus repo only, and engine source ships as a one-way git bundle from the
brain. Runbook updated to as-built in the closing PR. Known launch limitation
unchanged: signup per-IP cap is effectively global behind the proxy until
#0131's XFF handling ships.
