#!/usr/bin/env bash
# apply-branch-protection.sh — apply branch-protection.json to the git host.
#
#   scripts/apply-branch-protection.sh [--dry-run] [--flavor github|forgejo|gitea]
#                                      [--repo owner/name] [--server URL] [--token T]
#
# Reads the host-neutral branch-protection.json (config-as-code) and translates
# it to the host's API:
#   - github : a repository ruleset      (gh api repos/<repo>/rulesets)
#   - forgejo/gitea : a branch protection (POST/PATCH .../branch_protections)
# --dry-run prints the exact host payload instead of applying it.
#
# Auth:
#   github          : gh CLI must be authenticated (or GH_TOKEN set)
#   forgejo/gitea   : a token via --token or $FORGEJO_TOKEN/$GITEA_TOKEN with
#                     write access to the repo's branch protections
set -euo pipefail

flavor="forgejo"
dry=0 repo="" server="" token="${FORGEJO_TOKEN:-${GITEA_TOKEN:-}}"
while [ $# -gt 0 ]; do case "$1" in
  --dry-run) dry=1; shift;;
  --flavor)  flavor="$2"; shift 2;;
  --repo)    repo="$2"; shift 2;;
  --server)  server="$2"; shift 2;;
  --token)   token="$2"; shift 2;;
  *) echo "unknown arg: $1" >&2; exit 2;;
esac; done

here="$(cd "$(dirname "$0")" && pwd)"
cfg="$here/../branch-protection.json"
[ -f "$cfg" ] || { echo "no branch-protection.json beside the repo root (looked at $cfg)" >&2; exit 2; }

# Derive repo (owner/name) and server URL from the origin remote when not given.
origin="$(git -C "$here/.." remote get-url origin 2>/dev/null || true)"
if [ -z "$repo" ]; then
  repo="$(printf '%s' "$origin" | sed -E 's#^.*[:/]([^/]+/[^/]+?)(\.git)?$#\1#')"
fi
if [ -z "$server" ]; then
  server="$(printf '%s' "$origin" | sed -E 's#^(https?://)([^@/]*@)?([^/]+)/.*#\1\3#')"
fi

# The protected branch name, for the apply messages and the Forgejo PATCH URL.
# (Derived in bash too — it was previously only bound inside the python heredoc,
# so a real apply tripped `set -u` with "branch: unbound variable".)
branch="$(CFG="$cfg" python3 -c 'import json,os;print(json.load(open(os.environ["CFG"]))["branch"])')"

# Build the host payload from the neutral config (python3 stdlib).
payload="$(CFG="$cfg" FLAVOR="$flavor" python3 - <<'PY'
import json, os
cfg = json.load(open(os.environ["CFG"]))
flavor = os.environ["FLAVOR"]
branch = cfg["branch"]
checks = [c.strip() for c in str(cfg.get("required_checks","")).split(",") if c.strip()]
approvals = int(cfg.get("required_approvals", 0))
dismiss = bool(cfg.get("dismiss_stale_reviews", True))
linear = bool(cfg.get("require_linear_history", True))
noff = bool(cfg.get("block_force_push", True))
outdated = bool(cfg.get("block_on_outdated_branch", True))
admins = bool(cfg.get("apply_to_admins", True))

if flavor == "github":
    rules = [
        {"type": "pull_request", "parameters": {
            "required_approving_review_count": approvals,
            "dismiss_stale_reviews_on_push": dismiss,
            "require_code_owner_review": False,
            "require_last_push_approval": False,
            "required_review_thread_resolution": False}},
    ]
    if checks:
        rules.append({"type": "required_status_checks", "parameters": {
            "required_status_checks": [{"context": c} for c in checks],
            "strict_required_status_checks_policy": False}})
    if linear: rules.append({"type": "required_linear_history"})
    if noff:   rules.append({"type": "non_fast_forward"})
    out = {"name": f"protect-{branch}", "target": "branch", "enforcement": "active",
           "conditions": {"ref_name": {"include": [f"refs/heads/{branch}"], "exclude": []}},
           "rules": rules}
else:  # forgejo / gitea
    out = {"rule_name": branch, "branch_name": branch,
           "enable_status_check": bool(checks), "status_check_contexts": checks,
           "required_approvals": approvals, "dismiss_stale_approvals": dismiss,
           "block_on_outdated_branch": outdated, "apply_to_admins": admins,
           "require_signed_commits": False}
print(json.dumps(out, indent=2))
PY
)"

if [ "$dry" = "1" ]; then
  echo "# flavor=$flavor repo=$repo server=$server"
  echo "$payload"
  exit 0
fi

echo "applying branch protection for $branch on $repo ($flavor) ..."
if [ "$flavor" = "github" ]; then
  command -v gh >/dev/null || { echo "gh CLI not found (needed for github)" >&2; exit 2; }
  printf '%s' "$payload" | gh api --method POST "repos/$repo/rulesets" --input - \
    && echo "ruleset created" \
    || { echo "POST failed — a ruleset may already exist; reconcile in the repo settings or delete it first" >&2; exit 1; }
else
  [ -n "$token" ] || { echo "no token (set \$FORGEJO_TOKEN/\$GITEA_TOKEN or pass --token)" >&2; exit 2; }
  api="$server/api/v1/repos/$repo/branch_protections"
  # Create, or update if a protection for this branch already exists. Forgejo
  # returns 403 ("Branch protection already exist") for a duplicate, Gitea 409/422
  # — fall back to PATCH on all three (a genuine auth 403 then fails the PATCH too).
  code=$(printf '%s' "$payload" | curl -s -o /tmp/bp.out -w '%{http_code}' -X POST "$api" \
           -H "Authorization: token $token" -H "Content-Type: application/json" -d @-)
  if [ "$code" = "403" ] || [ "$code" = "409" ] || [ "$code" = "422" ]; then
    code=$(printf '%s' "$payload" | curl -s -o /tmp/bp.out -w '%{http_code}' -X PATCH "$api/$branch" \
             -H "Authorization: token $token" -H "Content-Type: application/json" -d @-)
  fi
  case "$code" in 2*) echo "branch protection applied ($code)";;
    *) echo "API call failed ($code):" >&2; cat /tmp/bp.out >&2; exit 1;; esac
fi
