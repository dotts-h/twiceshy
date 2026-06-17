#!/usr/bin/env bash
# sync-forgejo.sh [docs/issues/NNNN-*.md ...] — mirror local markdown issues to
# Forgejo/Gitea Issues via the REST API. The local file is canonical; the forge
# holds the live view, the conversation, and the milestone/epic roll-up.
#
# Per file: create the issue if its `forgejo:` field is empty (writing the index
# back), otherwise update title/body/state/labels in place. The `group:` field
# becomes an `epic-NNNN` label plus a "Part of epic …" header line; `depends_on:`
# becomes a "Blocked by …" header; `severity:` and epic-ness become labels; an
# optional `milestone:` field is resolved to its forge milestone by title.
#
# Idempotent: the full label set is rewritten each run, so re-syncing reconciles.
# Best-effort by design: no token / no network exits 0 with a notice — the local
# store stays the source of truth either way.
#
# Config (env, with sane fallbacks):
#   FORGEJO_API   default: <scheme://host> of `origin` + /api/v1
#   FORGEJO_REPO  default: owner/name parsed from `origin`
#   FORGEJO_TOKEN default: the token baked into git's url.insteadOf rewrite, if any
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

origin="$(git remote get-url origin 2>/dev/null || echo)"

# API base: derive <scheme://host>/api/v1 from an https origin unless overridden.
API="${FORGEJO_API:-}"
if [ -z "$API" ]; then
  host="$(printf '%s' "$origin" | sed -nE 's#^(https?://[^/]+)/.*#\1#p')"
  [ -n "$host" ] && API="$host/api/v1"
fi
[ -n "$API" ] || { echo "set FORGEJO_API (could not derive from origin) — skipping mirror (local store stays canonical)"; exit 0; }

# Repo (owner/name) from origin unless overridden.
if [ -z "${FORGEJO_REPO:-}" ]; then
  FORGEJO_REPO="$(printf '%s' "$origin" | sed -E 's#^.*[/:]([^/]+/[^/]+)$#\1#; s/\.git$//')"
fi

# Token: explicit env, else reuse one baked into a git url.insteadOf rewrite.
TOKEN="${FORGEJO_TOKEN:-}"
if [ -z "$TOKEN" ]; then
  TOKEN="$(git config --get-regexp '^url\..*insteadof$' 2>/dev/null \
            | grep -oE ':[0-9a-f]{32,}@' | head -1 | tr -d ':@')"
fi
[ -n "$TOKEN" ] || { echo "no Forgejo token (FORGEJO_TOKEN / git-config) — skipping mirror (local store stays canonical)"; exit 0; }

# Reachability check — never fail a dev-loop step because the forge is asleep.
curl -fsS -m 5 "$API/repos/$FORGEJO_REPO" -H "Authorization: token $TOKEN" >/dev/null 2>&1 \
  || { echo "Forgejo unreachable at $API — skipping mirror"; exit 0; }

files=("$@")
if [ ${#files[@]} -eq 0 ]; then
  while IFS= read -r f; do files+=("$f"); done \
    < <(find docs/issues -maxdepth 1 -name '[0-9][0-9][0-9][0-9]-*.md' | sort)
fi
[ ${#files[@]} -gt 0 ] || { echo "no issue files to sync"; exit 0; }

export FORGEJO_API="$API" FORGEJO_REPO FORGEJO_TOKEN="$TOKEN"
FILES_JOINED="$(printf '%s\n' "${files[@]}")" FILES="$FILES_JOINED" python3 - <<'PY'
import json, os, re, sys, urllib.request, urllib.error

API   = os.environ["FORGEJO_API"].rstrip("/")
REPO  = os.environ["FORGEJO_REPO"]
TOKEN = os.environ["FORGEJO_TOKEN"]
FILES = [f for f in os.environ["FILES"].splitlines() if f.strip()]

def api(method, path, body=None):
    url = f"{API}/repos/{REPO}{path}"
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"token {TOKEN}")
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            raw = r.read()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        sys.stderr.write(f"  ! {method} {path} -> {e.code} {e.read().decode(errors='replace')[:200]}\n")
        raise

# --- label & milestone caches (name -> id) -------------------------------------
labels = {l["name"]: l["id"] for l in api("GET", "/labels?limit=100")}
def label_id(name, color="#ededed"):
    if name not in labels:
        labels[name] = api("POST", "/labels", {"name": name, "color": color, "description": ""})["id"]
    return labels[name]

milestones = {m["title"]: m["id"] for m in api("GET", "/milestones?state=all&limit=100")}

def frontmatter(path):
    lines = open(path).read().splitlines()
    fm, seen = [], 0
    for ln in lines:
        if ln.strip() == "---":
            seen += 1
            if seen == 2: break
            continue
        if seen == 1: fm.append(ln)
    def field(name):
        for ln in fm:
            m = re.match(rf"\s*{name}:\s*(.*)$", ln)
            if m: return m.group(1).strip()
        return ""
    idx = [i for i, l in enumerate(lines) if l.strip() == "---"]
    body = "\n".join(lines[idx[1] + 1:]) if len(idx) >= 2 else ""
    return field, body

def write_back(path, num):
    txt = open(path).read()
    if re.search(r"(?m)^forgejo:", txt):
        txt = re.sub(r"(?m)^forgejo:.*$", f"forgejo: {num}", txt)
    else:  # add it right after the github: line (or after id: as a fallback)
        anchor = "github:" if re.search(r"(?m)^github:", txt) else "id:"
        txt = re.sub(rf"(?m)^({anchor}.*)$", rf"\1\nforgejo: {num}", txt, count=1)
    open(path, "w").write(txt)

for f in FILES:
    if not os.path.isfile(f):
        print(f"skip: {f} (not a file)"); continue
    field, raw_body = frontmatter(f)
    iid    = field("id")
    title  = field("title")
    if len(title) >= 2 and title[0] in "\"'" and title[-1] == title[0]:
        title = title[1:-1]  # strip YAML quoting so "Epic: …" is detected
    status = field("status") or "open"
    group  = field("group")
    fjnum  = field("forgejo")
    sev    = field("severity")
    msname = field("milestone")
    deps   = re.findall(r"\d+", field("depends_on") or "")
    is_epic = title.lower().startswith("epic:")

    # Header lines so an issue never reads isolated, then the canonical body.
    header = []
    if group: header.append(f"Part of epic {group}.")
    if deps:  header.append("Blocked by: " + ", ".join(deps) + ".")
    body = ((" ".join(header) + "\n\n") if header else "") + raw_body.strip() + \
           f"\n\n_Mirrored from `{f}` — the markdown file is canonical._"

    # Deterministic label set (rewritten every sync → idempotent).
    names = ["epic" if is_epic else "feature"]
    if sev:    names.append(f"severity/{sev}")
    if group:  names.append(f"epic-{group}")
    if status == "in-progress": names.append("in-progress")
    if deps:   names.append("blocked")
    label_ids = [label_id(n, "#a64dff" if n.startswith("epic") else "#ededed") for n in names]

    payload = {"title": f"[{iid}] {title}", "body": body}
    if msname:
        if msname in milestones: payload["milestone"] = milestones[msname]
        else: sys.stderr.write(f"  ! milestone '{msname}' not found; leaving unset\n")

    if not fjnum:
        payload["labels"] = label_ids
        issue = api("POST", "/issues", payload)
        num = issue["number"]
        write_back(f, num)
        if status == "closed":
            api("PATCH", f"/issues/{num}", {"state": "closed"})
        print(f"created #{num} for {iid}")
    else:
        payload["state"] = "closed" if status == "closed" else "open"
        api("PATCH", f"/issues/{fjnum}", payload)
        api("PUT", f"/issues/{fjnum}/labels", {"labels": label_ids})
        print(f"updated #{fjnum} for {iid}")
PY
