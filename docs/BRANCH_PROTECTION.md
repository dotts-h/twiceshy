# Branch protection — twiceshy

CONVENTIONS says *branch → PR → merge, CI green before merge*. That's a wish
until the host enforces it. This recipe ships the enforcement as code:

- **`branch-protection.json`** — the desired state (host-neutral): the protected
  branch, the required status checks, review approvals, stale-review dismissal,
  linear history, no force-push.
- **`scripts/apply-branch-protection.sh`** — translates it to the host's API
  (GitHub *rulesets*; Forgejo/Gitea *branch_protections*) and applies it.

## Apply it (the backfill step)

The installer can't call the host API during adoption — applying is a one-time
(idempotent) step you run with credentials:

```sh
# See exactly what will be sent, first:
scripts/apply-branch-protection.sh --dry-run

# GitHub (uses an authenticated gh CLI):
scripts/apply-branch-protection.sh --flavor github

# Forgejo/Gitea (needs a token with repo write):
FORGEJO_TOKEN=... scripts/apply-branch-protection.sh --flavor forgejo
```

Re-running reconciles: Forgejo updates the existing rule; on GitHub, delete the
old ruleset first (or edit it) if a `protect-main` ruleset exists.

## Getting `required_checks` right

`required_checks` (in `branch-protection.json`) must match the **exact** status
names CI publishes — not the workflow name, the *check* name. Open a recent PR's
checks list and copy the names verbatim (e.g. the quality recipe's job, the
`e2e` recipe's job). A required check that never reports **blocks every PR**, so
verify with a real PR after applying.

## Notes

- **Solo repos:** set `required_approvals` to `0` but keep the status-check gate
  — you still get "CI must be green to merge" without needing a second reviewer.
- **Docs-only PRs:** the quality/e2e workflows `paths-ignore` docs, so a
  required check may not report on a docs-only PR. Confirm the host treats a
  skipped required check as passing, or such PRs hang unmergeable.
- This is config-as-code: change `branch-protection.json`, re-run the script.
