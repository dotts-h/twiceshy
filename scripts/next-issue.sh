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
import re, sys, pathlib

idx = pathlib.Path("docs/issues/INDEX.md")
if not idx.exists():
    print("no docs/issues/INDEX.md — install the issues recipe first, or run from the repo root.")
    sys.exit(2)
lines = idx.read_text().splitlines()

def rows(after_header_contains):
    """Yield cell-lists for markdown table rows after a header line matching the marker."""
    started = False
    for ln in lines:
        if not started:
            if ln.strip().startswith("|") and after_header_contains in ln:
                started = True
            continue
        if not ln.strip().startswith("|"):
            if started: break
            continue
        cells = [c.strip() for c in ln.strip().strip("|").split("|")]
        if set("".join(cells)) <= set("-: "):  # separator row
            continue
        yield cells

def idnum(cell):
    m = re.search(r"\[(\d+)\]", cell) or re.search(r"(\d+)", cell)
    return m.group(1) if m else cell

# --- parse epics table (| id | title | status | children |)
epics = {}
for c in rows("children"):
    if len(c) < 4: continue
    eid = idnum(c[0])
    kids = [k.strip() for k in re.split(r"[,\s]+", c[-1]) if k.strip() and k.strip() != "—"]
    epics[eid] = {"title": c[1], "status": c[2].lower(), "children": kids}

# --- parse issues table (| id | title | status | severity | group | links |)
# Epics may be filed in THIS table (title starts "Epic:") rather than the epics
# table above — recognize them either way.
issues = {}
inline_epics = {}
for c in rows("severity"):
    if len(c) < 5: continue
    iid = idnum(c[0])
    grp = idnum(c[4]) if c[4] and c[4] != "—" else ""
    rec = {"title": c[1], "status": c[2].lower(), "severity": c[3].lower(), "group": grp}
    issues[iid] = rec
    if re.match(r"\s*Epic\b", c[1]) and iid not in epics:
        inline_epics[iid] = {"title": c[1], "status": c[2].lower(), "children": []}

def is_open(s): return s not in ("closed", "done", "shipped", "resolved")

# --- depends_on edges, read from each issue file's frontmatter --------------
def load_depends(iid):
    """Return the list of issue ids `iid` is blocked by (depends_on)."""
    matches = list(pathlib.Path("docs/issues").glob(f"{iid}-*.md"))
    if not matches: return []
    txt = matches[0].read_text().splitlines()
    fm, seen = [], 0
    for ln in txt:
        if ln.strip() == "---":
            seen += 1
            if seen == 2: break
            continue
        if seen == 1: fm.append(ln)
    deps = []
    for i, ln in enumerate(fm):
        m = re.match(r"\s*depends_on:\s*(.*)$", ln)
        if not m: continue
        inline = m.group(1).strip()
        if inline and inline not in ("[]", "~", "null"):
            deps += re.findall(r"\d+", inline)            # depends_on: [0001, 0002]
        else:                                              # block list form
            for ln2 in fm[i+1:]:
                b = re.match(r"\s*-\s*(\d+)", ln2)
                if b: deps.append(b.group(1))
                elif ln2.strip() and not ln2.startswith((" ", "\t")): break
        break
    return [f"{int(d):04d}" for d in deps]

depends = {i: load_depends(i) for i in issues}
def open_blockers(iid):
    return [d for d in depends.get(iid, []) if d in issues and is_open(issues[d]["status"])]

all_epics = dict(epics); all_epics.update(inline_epics)
open_epics = {e: v for e, v in all_epics.items() if is_open(v["status"])}
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
            flags.append(f"[!] issue {i} depends_on {d}, which is not in the index — dangling edge.")
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
