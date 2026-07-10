---
id: 0149
title: sync-forgejo.sh derives the API base with userinfo credentials left in — crashes on token-embedded origin URLs
status: open
severity: medium
group: 
depends_on: []
forgejo: 607
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary
`scripts/sync-forgejo.sh` derives the API base from the origin remote with
`sed -nE 's#^(https?://[^/]+)/.*#\1#p'`. `[^/]+` keeps the userinfo, so an origin like
`http://claude:<token>@192.168.50.244:3030/claude/twiceshy.git` yields
`API=http://claude:<token>@192.168.50.244:3030/api/v1`, and the script's Python
urllib helper dies with `URLError [Errno -2] Name or service not known` (it treats
`claude:<token>@192.168.50.244` as the hostname). Token-embedded origins are exactly
how the brain clones this repo, so the mirror sync has been silently broken here:
issues 0141–0148 were never mirrored until a manual `FORGEJO_API=…` override run on
2026-07-10 backfilled them (forgejo #584–#591).

## Repro
1. `git remote set-url origin http://user:tok@host:3030/owner/repo.git`
2. run `scripts/sync-forgejo.sh` with no `FORGEJO_API` set
Expected: API base derived as `http://host:3030/api/v1` (userinfo stripped; better yet,
reuse the embedded token when `FORGEJO_TOKEN` is unset — the token-reuse branch below
already greps it from git config, not the URL).
Actual: urllib crash `Name or service not known`; no graceful skip, no mirror sync.

## Evidence
2026-07-10 session: traceback from `scripts/sync-forgejo.sh` ending
`urllib.error.URLError: <urlopen error [Errno -2] Name or service not known>`; rerun with
`FORGEJO_API="http://192.168.50.244:3030/api/v1"` synced cleanly and created #584–#591.

## Notes
Fix shape: strip `userinfo@` when deriving (`s#^(https?://)([^/@]+@)?([^/]+)/.*#\1\3#p`)
plus a `scripts/sync-forgejo.test.sh` case for a token-embedded origin. Fail-open contract
(the "skipping mirror" paths) should also catch the helper's non-zero exit so a bad derive
degrades to skip instead of a traceback.
