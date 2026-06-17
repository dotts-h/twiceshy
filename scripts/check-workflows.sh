#!/usr/bin/env bash
# check-workflows.sh — guard the CI/release workflow invariants that regress
# silently, so they can't land. Deterministic; wire it into the CI lint job AND
# the local lint gate. Exits non-zero with a clear message on any violation.
#
# Invariants enforced (portable to GitHub Actions AND Forgejo/Gitea Actions —
# scans .github/workflows, .forgejo/workflows, .gitea/workflows):
#   1. No feature-branch CI double-runs. A workflow triggered on `push` must list
#      ONLY the default branch under push.branches — listing a feature branch glob
#      runs the whole pipeline twice per push (push AND pull_request both fire).
#      Tag-triggered workflows (push.tags without push.branches) are exempt.
#   2. Release version must resolve from the dispatch input, not the branch ref.
#      `${GITHUB_REF_NAME:-input}` shadows the input on workflow_dispatch (the ref
#      is the *branch*, non-empty), publishing a release tagged after the branch.
#      The correct form: ${{ github.event.inputs.tag || github.ref_name }}.
#
# Default branch: $DEFAULT_BRANCH env, else origin/HEAD, else "main".
set -uo pipefail
cd "$(git rev-parse --show-toplevel 2>/dev/null || echo .)" || exit 2

# Discover every Actions workflow dir this host might run.
wf_dirs=()
for d in .github/workflows .forgejo/workflows .gitea/workflows; do
  [ -d "$d" ] && wf_dirs+=("$d")
done
if [ "${#wf_dirs[@]}" -eq 0 ]; then
  echo "no workflow dirs (.github/.forgejo/.gitea) — nothing to check"
  exit 0
fi

branch="${DEFAULT_BRANCH:-}"
if [ -z "$branch" ]; then
  branch="$(git symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null | sed 's|^origin/||')"
fi
branch="${branch:-main}"

fail=0

# Rule 1 — push.branches must be exactly [<default branch>].
if ! DEFAULT_BRANCH="$branch" WF_DIRS="${wf_dirs[*]}" python3 - <<'PY'; then
import glob, os, re, sys

default = os.environ["DEFAULT_BRANCH"]
files = []
for d in os.environ["WF_DIRS"].split():
    files += glob.glob(f"{d}/*.yml") + glob.glob(f"{d}/*.yaml")
files = sorted(files)
bad = []

def branches_via_yaml(path):
    import yaml
    wf = yaml.safe_load(open(path)) or {}
    on = wf.get("on", wf.get(True, {}))  # bare `on:` parses as boolean True (YAML 1.1)
    if not isinstance(on, dict):
        return None
    push = on.get("push")
    if isinstance(push, dict) and "branches" in push:
        return [str(b) for b in (push["branches"] or [])]
    return None

def branches_via_text(path):
    """Fallback when PyYAML is absent: find the push: block under on: and read
    its branches: list (inline [a, b] or block '- a' form)."""
    lines = open(path).read().splitlines()
    in_push = push_indent = None
    branches = None
    i = 0
    while i < len(lines):
        raw = lines[i]
        line = re.sub(r"#.*$", "", raw)
        s, indent = line.strip(), len(line) - len(line.lstrip())
        if in_push is None:
            if re.match(r"push:\s*$", s):
                in_push, push_indent = True, indent
        else:
            if s and indent <= push_indent:
                break  # left the push block
            m = re.match(r"branches:\s*(.*)$", s)
            if m:
                val = m.group(1).strip()
                if val.startswith("["):
                    branches = [b.strip().strip("'\"") for b in val[1:-1].split(",") if b.strip()]
                else:
                    branches = []
                    j = i + 1
                    while j < len(lines):
                        s2 = re.sub(r"#.*$", "", lines[j]).strip()
                        if s2.startswith("- "):
                            branches.append(s2[2:].strip().strip("'\""))
                            j += 1
                        elif not s2:
                            j += 1
                        else:
                            break
                break
        i += 1
    return branches

for f in files:
    try:
        try:
            got = branches_via_yaml(f)
        except ImportError:
            got = branches_via_text(f)
    except Exception as e:
        print(f"ERROR: {f}: unparseable workflow: {e}")
        sys.exit(2)
    if got is None:
        continue
    extra = [b for b in got if b != default]
    if extra:
        bad.append((f, extra))

for f, extra in bad:
    print(f"ERROR: {f}: push.branches must be [{default}]; drop {extra} — "
          f"a feature-branch push trigger doubles every CI run "
          f"(push + pull_request both fire).")
sys.exit(1 if bad else 0)
PY
  fail=1
fi

# Rule 2 — release version resolution must not shadow the dispatch input.
# Strip comment lines first so a workflow documenting the buggy form doesn't trip.
for d in "${wf_dirs[@]}"; do
  for wf in "$d"/*.yml "$d"/*.yaml; do
    [ -e "$wf" ] || continue
    if grep -v '^[[:space:]]*#' "$wf" | grep -q 'GITHUB_REF_NAME:-'; then
      echo "ERROR: $wf resolves a version with \${GITHUB_REF_NAME:-…}; on" \
           "workflow_dispatch this tags the release after the *branch*." \
           "Use \${{ github.event.inputs.tag || github.ref_name }} instead."
      fail=1
    fi
  done
done

if [ "$fail" -ne 0 ]; then
  echo "workflow checks FAILED" >&2
  exit 1
fi
echo "workflow checks passed (default branch: $branch; dirs: ${wf_dirs[*]})"
