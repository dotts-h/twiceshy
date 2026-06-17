# Issue file format — docs/issues/NNNN-kebab-title.md

```markdown
---
id: 0007
title: Topbar overflows at tablet width
status: open            # open | in-progress | closed
severity: medium        # low | medium | high | critical
group:                  # epic id if connected; empty if isolated
depends_on: []          # issue ids that MUST land first (blocked-by edges); [] if none
github:                 # issue number, filled by sync-github.sh (github host)
forgejo:                # issue number, filled by sync-forgejo.sh (forgejo/gitea host)
links:
  adr:                  # decision record if one caused/relates to it
  prs: []               # PRs that address it
  issues: []            # related issue ids (theme/siblings — NON-directional)
  regression:           # REGRESSIONS.md entry once guarded
assets: []              # screenshots/logs under docs/issues/assets/
---

## Summary
One or two sentences: what's wrong (or wanted) and where.

## Repro
1. step
Expected: …
Actual: …

## Evidence
(screenshot / log link — mandatory for bugs)

## Notes
Root-cause hypotheses, scope, links to the decision/learning that gives context.
```

## Rules

- **Evidence is mandatory for bugs.** A repro and (for anything visual) a
  screenshot. No repro → it will be ignored or closed unactioned.
- **id is zero-padded and monotonic**, matching the filename prefix.
- **links are the point** — an issue with no links is a sticky note. Attach the
  decision (why), the PR (fix), the regression (guard), and sibling issues (theme).
- **`depends_on` is the dependency graph, not a theme link.** Use it only for
  *hard* ordering — "this cannot start until that lands". It is directional and
  machine-read: the next-issue picker won't recommend an item whose `depends_on`
  is still open, and the set of unblocked items with disjoint seams is exactly
  what can be built in parallel. Keep "see also" in `links.issues`; never cycle.
- **Closing an epic closes its body too**: tick the epic's own acceptance boxes
  and fill its links, not just status + INDEX + children.
