---
id: 0124
title: Epic: Public alpha — hosted multi-tenant remote-MCP service (ADR-0030)
status: open
severity: high
group: 
depends_on: []
forgejo:
links:
  adr: ADR-0030
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

Epic for ADR-0030: ship twiceshy's public alpha as ONE hosted multi-tenant
instance, delivered exclusively as a remote MCP endpoint (streamable HTTP the
server already speaks + per-user bearer tokens). No client app: every major
coding agent is a remote-MCP client; onboarding is one `claude mcp add -t http`
line with a token. Pull-only at launch (search/get); the write path opens as a
fast follow behind a low-trust origin tier. Push-channel hosting, team corpuses,
billing and OAuth are explicitly out of alpha scope.

Children: #0125 (token layer), #0126 (per-tenant telemetry + dashboard), #0127
(landing page + signup), #0128 (write-path hardening), #0129 (off-homelab
deploy), #0130 (data-license terms). Launch gate = 0125+0126+0127+0129+0130;
0128 gates opening the write tools, not the launch.

## Status 2026-07-06

Children 0125, 0126, 0127, 0128, and 0130 closed same-day (ADR-0030 alpha
build day). 0129 (off-homelab deploy) remains open — shipped its deploy
artifacts and runbook but is blocked on two operator decisions (hosting
provider, domain). The launch gate is now just those two decisions plus
going live per the runbook.

## Notes

Cold-start honesty (ADR-0030 consequences): the measurement chain (#0067/#0069,
issues #0417-class work) stays priority one in parallel — the flywheel only
spins if serving demonstrably helps. This epic is buildable independently of
those results.
