#!/usr/bin/env bash
# next-issue.sh — read docs/issues/ and recommend the next thing to build.
#
# Precedence:
#   1) an OPEN leaf issue, parent epic OPEN, all depends_on CLOSED -> BUILD it
#      (an open child with an OPEN blocker is BLOCKED — listed, not recommended)
#   2) an OPEN epic with NO child issues yet                       -> break it down, file child #1
#   3) an OPEN epic whose children are ALL closed                  -> STALE: close it (human call)
#   4) nothing open                                                -> roadmap research pass
#
# Reads depends_on edges from each issue file to (a) skip blocked items and
# (b) surface the set buildable *now* — which, with disjoint seams, is exactly
# what can run in parallel. Reconciles epic-vs-child status and flags drift.
# Prints a ranked recommendation to stdout; does not mutate anything.
# Language-agnostic: needs only git, bash, python3 (stdlib).
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
exec python3 - "$@" <<'PY'
import json, re, sys, pathlib

issue_dir = pathlib.Path("docs/issues")
if not issue_dir.is_dir():
    print("no docs/issues/ — install the issues recipe first, or run from the repo root.")
    sys.exit(2)

def unquote(value):
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] == '"':
        try: return json.loads(value)
        except json.JSONDecodeError: return value[1:-1]
    if len(value) >= 2 and value[0] == value[-1] == "'":
        return value[1:-1].replace("''", "'")
    return "" if value.lower() in ("null", "~") else value

def load_issue(path):
    """Read the picker fields directly from canonical issue frontmatter."""
    lines = path.read_text().splitlines()
    fm, seen = [], 0
    for line in lines:
        if line.strip() == "---":
            seen += 1
            if seen == 2: break
            continue
        if seen == 1: fm.append(line)
    fields = {}
    for line in fm:
        match = re.match(r"^([A-Za-z_][A-Za-z0-9_]*):\s*(.*)$", line)
        if match: fields[match.group(1)] = unquote(match.group(2))
    issue_id = fields.get("id", path.name[:4])
    deps = []
    for pos, line in enumerate(fm):
        match = re.match(r"^depends_on:\s*(.*)$", line)
        if not match: continue
        inline = match.group(1).strip()
        if inline and inline not in ("[]", "~", "null"):
            deps.extend(re.findall(r"\d+", inline))
        else:
            for child in fm[pos + 1:]:
                item = re.match(r"\s*-\s*(\d+)", child)
                if item: deps.append(item.group(1))
                elif child.strip() and not child.startswith((" ", "\t")): break
        break
    return issue_id, {
        "title": fields.get("title", ""),
        "status": fields.get("status", "open").lower(),
        "severity": fields.get("severity", "medium").lower(),
        "group": fields.get("group", ""),
        "depends": [f"{int(dep):04d}" for dep in deps],
    }

issues = {}
for path in sorted(issue_dir.glob("[0-9][0-9][0-9][0-9]-*.md")):
    issue_id, record = load_issue(path)
    issues[issue_id] = record
epics = {
    issue_id: {"title": record["title"], "status": record["status"], "children": []}
    for issue_id, record in issues.items()
    if record["title"].lstrip().lower().startswith("epic:")
}

def is_open(s): return s not in ("closed", "done", "shipped", "resolved")

depends = {issue_id: record["depends"] for issue_id, record in issues.items()}
def open_blockers(iid):
    return [d for d in depends.get(iid, []) if d in issues and is_open(issues[d]["status"])]

open_epics = {e: v for e, v in epics.items() if is_open(v["status"])}
sev_rank = {"critical": 0, "high": 1, "medium": 2, "low": 3}
recs, flags, blocked, buildable = [], [], [], []

for eid, ev in sorted(open_epics.items()):
    # children = those listed on the epic ∪ issues whose group back-references it
    kids = sorted(set([k for k in ev["children"] if k in issues] +
                      [i for i, iv in issues.items() if iv["group"] == eid]), key=int)
    open_kids = [k for k in kids if is_open(issues[k]["status"])]
    if not kids:
        recs.append((1, eid, f"[2] OPEN epic {eid} has NO child issues yet — break it down and FILE child #1.\n      epic: {ev['title']}\n      -> scripts/new-issue.sh \"<first slice>\" --group {eid} [--depends <id>]"))
    elif open_kids:
        for k in sorted(open_kids, key=lambda k: (sev_rank.get(issues[k]["severity"], 9), int(k))):
            blk = open_blockers(k)
            if blk:
                blocked.append((k, eid, blk))
            else:
                buildable.append((k, eid))
    else:
        flags.append(f"[3] STALE: epic {eid} is OPEN but every child is closed — close the epic, or it has un-filed follow-ups. Human call.\n      epic: {ev['title']}")

# Cycle / dangling-edge sanity (best-effort; dedup the symmetric cycle pair).
seen_cycles = set()
for i in issues:
    for d in depends.get(i, []):
        if d not in issues:
            flags.append(f"[!] issue {i} depends_on {d}, which has no canonical issue file — dangling edge.")
        elif i in depends.get(d, []):
            pair = tuple(sorted((i, d)))
            if pair not in seen_cycles:
                seen_cycles.add(pair)
                flags.append(f"[!] dependency CYCLE between {pair[0]} and {pair[1]} — break it before building either.")

buildable.sort(key=lambda t: (sev_rank.get(issues[t[0]]["severity"], 9), int(t[0])))
for k, eid in buildable:
    recs.append((0, k, f"[1] BUILD issue {k} (epic {eid}, sev {issues[k]['severity']}, unblocked).\n      {issues[k]['title']}\n      -> read docs/issues/{k}-*.md, then branch + test-first."))

print("# Next-item recommendation (from docs/issues/)\n")
if recs:
    for _, _, msg in sorted(recs, key=lambda r: (r[0], int(r[1]))):
        print("  " + msg + "\n")
else:
    print("  [4] No open epics with actionable work. Run a roadmap research pass to seed the next milestone.\n")

if buildable:
    ids = ", ".join(k for k, _ in buildable)
    print("## Parallelizable now (unblocked)\n")
    print(f"  These have no open blocker: {ids}.")
    print("  Any subset touching DISJOINT seams can run as parallel lanes (compare each")
    print("  issue's Summary/Touches lines). See docs/DEV_LOOP.md 'Parallel lanes'.\n")
if blocked:
    print("## Blocked (don't start — finish the blocker first)\n")
    for k, eid, blk in sorted(blocked, key=lambda t: int(t[0])):
        print(f"  issue {k} (epic {eid}) is BLOCKED by open: {', '.join(blk)} — {issues[k]['title']}\n")
if flags:
    print("## Flags (reconcile before picking)\n")
    for f in flags: print("  " + f + "\n")
if pathlib.Path("docs/ROADMAP.md").exists():
    print("Tip: also read docs/ROADMAP.md — depends_on encodes hard order; the roadmap adds the value ranking.")
PY
