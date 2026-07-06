---
id: 0127
title: Landing page: product explainer, self-serve token signup, terms acceptance
status: closed
severity: medium
group: 0124
depends_on: []
forgejo:
links:
  adr:
  prs: [512, 513, 519]
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

## Close-out (2026-07-06, PRs #512, #513, #519)

Shipped across three PRs: #512 (signup endpoint + v1 page), #513 (v2
redesign), #519 (v3 lean page + docs.html + terms.html — this PR also
restores PR #514, which had merged an empty diff after a branch-ref clobber;
see exp-4457 for the postmortem). The live-demo fast-follow (a real
search_experience transcript on the page) is tracked as #0132.
