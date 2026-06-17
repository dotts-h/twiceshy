#!/usr/bin/env bash
# start-fresh.sh — verify the base, then branch fresh from a trustworthy default branch.
#
#   start-fresh.sh <new-branch> [--expect-sha <sha>] [--require <path>[,<path>...]]
#
# Encodes the "verify the base before branching" rule so a stale remote (or a tag
# shadowing the branch name) fails LOUD, not silent:
#   - never resolves the bare branch name (a tag with the same name shadows it);
#     always branches from the fully-qualified refs/remotes/origin/<branch>.
#   - fast-forwards the local default branch (best-effort) and prints the SHA.
#   - optionally asserts the tip SHA and that named foundation files exist on it.
# Exit non-zero on any mismatch; the caller must stop on failure.
#
# Default branch: $DEFAULT_BRANCH env, else origin/HEAD, else "main".
set -euo pipefail

branch="${1:-}"; shift || true
expect_sha=""; require=""
while [ $# -gt 0 ]; do case "$1" in
  --expect-sha) expect_sha="$2"; shift 2;;
  --require)    require="$2";   shift 2;;
  *) echo "unknown arg: $1" >&2; exit 2;;
esac; done
[ -n "$branch" ] || { echo 'usage: start-fresh.sh <new-branch> [--expect-sha <sha>] [--require a,b]' >&2; exit 2; }

note() { printf '  %s\n' "$*"; }
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

echo "==> fetch origin (prune)"
git fetch origin --prune --tags --quiet

base="${DEFAULT_BRANCH:-}"
if [ -z "$base" ]; then
  # origin/HEAD may be unset (fresh or bare-cloned remotes) — that's fine, fall back.
  base="$(git symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null | sed 's|^origin/||' || true)"
fi
base="${base:-main}"

# A tag sharing the default branch's name shadows the branch in ambiguous refs —
# warn so the human knows why we never write the bare name.
if git rev-parse -q --verify "refs/tags/$base" >/dev/null 2>&1; then
  note "heads-up: a tag named '$base' exists and shadows the branch — using refs/remotes/origin/$base explicitly."
fi

remote_tip="$(git rev-parse "refs/remotes/origin/$base" 2>/dev/null)" \
  || fail "no refs/remotes/origin/$base — is the remote reachable, and is '$base' the default branch? (set DEFAULT_BRANCH to override)"
echo "==> origin/$base is $remote_tip"

# Assertions FIRST, against the remote ref — so a wrong/stale base fails loud
# *before* we touch any branch (never strand the caller mid-ritual).
if [ -n "$expect_sha" ]; then
  case "$remote_tip" in
    "$expect_sha"*) note "SHA assertion ok ($expect_sha)";;
    *) fail "origin/$base is $remote_tip but --expect-sha was $expect_sha (stale fetch?)";;
  esac
fi
if [ -n "$require" ]; then
  IFS=',' read -ra paths <<< "$require"
  for p in "${paths[@]}"; do
    p="$(echo "$p" | xargs)"; [ -n "$p" ] || continue
    git cat-file -e "$remote_tip:$p" 2>/dev/null \
      && note "foundation present: $p" \
      || fail "expected foundation file missing on origin/$base: $p (wrong base — branch was likely cut before it merged)"
  done
fi

# Best-effort fast-forward of the local default branch (assertions already passed).
# Skipped inside a LINKED WORKTREE, where the default branch is checked out in the
# primary worktree and a switch here would fail (git-dir != git-common-dir only there).
git_dir="$(git rev-parse --git-dir)"
common_dir="$(git rev-parse --git-common-dir)"
if [ "$git_dir" != "$common_dir" ]; then
  note "linked worktree detected — skipping the local '$base' fast-forward; the new branch is cut from origin/$base directly."
elif git show-ref -q --verify "refs/heads/$base"; then
  start_ref="$(git symbolic-ref -q --short HEAD || git rev-parse HEAD)"
  git switch "$base" --quiet 2>/dev/null || true
  git merge --ff-only "$remote_tip" --quiet 2>/dev/null \
    || note "local '$base' is not a fast-forward of origin/$base — cutting from origin/$base directly."
  git switch "$start_ref" --quiet 2>/dev/null || true
fi

echo "==> create branch '$branch' from origin/$base"
if git show-ref -q --verify "refs/heads/$branch"; then
  fail "branch '$branch' already exists locally — pick a fresh name or delete it first."
fi
git switch -c "$branch" "$remote_tip" --quiet
echo "==> on $(git rev-parse --abbrev-ref HEAD) @ $(git rev-parse --short HEAD) (fresh from origin/$base)"
