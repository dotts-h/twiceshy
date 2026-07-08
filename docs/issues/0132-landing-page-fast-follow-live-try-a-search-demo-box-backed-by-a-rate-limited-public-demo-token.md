---
id: 0132
title: Landing page fast-follow: live try-a-search demo box backed by a rate-limited public demo token
status: open
severity: low
group: 0124
depends_on: []
forgejo: 533
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
The landing hero demo is a scripted animation (honest, captioned as such). The most convincing possible element would be a real one: a "try a search" input that runs an actual search_experience against the live corpus and renders the returned cards to the visitor.
## Notes
Needs: a public demo tenant token baked server-side behind a /demo-search endpoint (never expose the token client-side), aggressive per-IP rate limiting, query-length caps, and no write access. Do after #0129 (deploy) — pointless on LAN. Parent: #0124. Raised during the 2026-07-06 v2 review ("is the demo live?").
