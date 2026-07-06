---
id: 0127
title: Landing page: product explainer, self-serve token signup, terms acceptance
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

Public landing page: what twiceshy is (validated engineering traps served to
agents at decision time), a live sample card, the one-line MCP onboarding
snippet, self-serve token signup (email → token, terms checkbox wired to #0130),
and links to the AGPL repo. Static site + one tiny signup endpoint (which calls
#0125 token issuance). Hosted with #0129.

## Notes

The page is also the product-review surface for prospective users — show a real
search_experience round-trip (asciinema or copy-paste transcript), not
marketing prose.
