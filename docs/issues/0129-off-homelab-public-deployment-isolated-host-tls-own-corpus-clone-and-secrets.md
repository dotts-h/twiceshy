---
id: 0129
title: Off-homelab public deployment: isolated host, TLS, own corpus clone and secrets
status: open
severity: medium
group: 0124
depends_on: []
forgejo:
links:
  adr:
  prs: []
  issues: []
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
