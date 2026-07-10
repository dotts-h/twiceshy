---
id: 0132
title: Landing page fast-follow: live try-a-search demo box backed by a rate-limited public demo token
status: closed
severity: low
group: 0124
depends_on: []
forgejo: 533
links:
  adr:
  prs: [558, 559]
  issues: []
  regression:
assets: []
---

## Summary
The landing hero demo is a scripted animation (honest, captioned as such). The most convincing possible element would be a real one: a "try a search" input that runs an actual search_experience against the live corpus and renders the returned cards to the visitor.
## Notes
Needs: a public demo tenant token baked server-side behind a /demo-search endpoint (never expose the token client-side), aggressive per-IP rate limiting, query-length caps, and no write access. Do after #0129 (deploy) — pointless on LAN. Parent: #0124. Raised during the 2026-07-06 v2 review ("is the demo live?").

## Close-out (2026-07-10)
Shipped in PR #558 (rate-limited `GET /demo-search` + landing try-a-search box + Caddy proxy; `internal/server/demo_search.go` uses a synthetic `demo` tenant id server-side — no real token exists, so none can leak client-side; per-IP 20/day + global 500/day caps, query-length cap, read-only pull path) and PR #559 (`TWICESHY_DEMO=1` on the public-alpha compose profile). Verified live 2026-07-10: `https://twiceshy.app/demo-search?q=fts5+match+syntax+error` returns real corpus hits (exp-0001 first). This file had drifted stale-open after the PRs merged — tracker-drift class tracked as #0141.
