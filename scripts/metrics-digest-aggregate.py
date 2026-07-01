#!/usr/bin/env python3
"""metrics-digest-aggregate.py — the pure aggregation core for the daily metrics
digest (#0116).

Extracted out of metrics-digest.sh so the numeric work is a hermetic unit under
scripts/metrics-digest.test.sh (fixture JSONL/db in, computed lines out) with no
Go toolchain, network, or live NAS/journalctl needed. metrics-digest.sh does the
collection (ssh + docker cp) and composes the final ntfy body around whatever
this prints; this script does no network I/O of its own.

Reads (both optional, independently):
  --gate-decisions PATH   the #0067 gate-decision JSONL log (already docker cp'd
                          out of the container by the caller). A MISSING path is
                          a collection failure and prints an ERROR line for the
                          section (never silently omitted — exp-0746/#0072: a
                          silent digest is the failure mode this repo keeps
                          re-learning). An EXISTING-but-empty file is a
                          legitimate "no traffic" answer, not an error.
  --usage-db PATH         the derived SQLite index (`usage` table: record_id,
                          retrieved, pushed, confirmed_helpful). Same
                          missing-vs-empty distinction. Opened read-only (URI
                          mode) so a missing file errors instead of sqlite3's
                          usual silent create-on-connect.
  --hours N               only decisions with ts within the last N hours count
                          (default 24).
  --now ISO8601           override "now" for deterministic tests; default is
                          the real current UTC time.

Prints one digest section per input given, in prose lines metrics-digest.sh
folds directly into the ntfy body.
"""
import argparse
import json
import os
import sqlite3
import sys
from datetime import datetime, timedelta, timezone


def parse_ts(ts):
    if not ts:
        return None
    try:
        return datetime.fromisoformat(ts.replace("Z", "+00:00"))
    except ValueError:
        return None


def rate(n, d):
    return (100.0 * n / d) if d else 0.0


def load_decisions(path, cutoff):
    """Return decisions with ts >= cutoff, or None if the file is missing/unreadable
    (a collection failure) — distinct from an existing-but-empty file (== [])."""
    if not os.path.exists(path):
        return None
    decisions = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                d = json.loads(line)
            except ValueError:
                continue
            t = parse_ts(d.get("ts"))
            if t is None or t < cutoff:
                continue
            decisions.append(d)
    return decisions


def push_search_lines(decisions):
    lines = []
    push = [d for d in decisions if d.get("channel") == "push"]
    served = [d for d in push if d.get("count", 0) > 0]
    lines.append(f"push: {len(push)} queries, {len(served)} served ({rate(len(served), len(push)):.1f}%)")

    triggers = {}
    for d in push:
        trig = d.get("trigger")
        if trig is None:
            continue
        b = triggers.setdefault(trig, {"push": 0, "served": 0})
        b["push"] += 1
        if d.get("count", 0) > 0:
            b["served"] += 1
    for trig in sorted(triggers):
        b = triggers[trig]
        lines.append(f"  trigger={trig}: {b['push']} queries, {b['served']} served ({rate(b['served'], b['push']):.1f}%)")

    id_counts = {}
    for d in served:
        for hit in d.get("served") or []:
            rid = hit.get("id")
            if rid:
                id_counts[rid] = id_counts.get(rid, 0) + 1
    top5 = sorted(id_counts.items(), key=lambda kv: (-kv[1], kv[0]))[:5]
    if top5:
        lines.append("  top served: " + ", ".join(f"{rid}x{n}" for rid, n in top5))

    samples = []
    for d in served:
        qt = d.get("query_text")
        if qt:
            samples.append(qt[:80])
        if len(samples) == 3:
            break
    if samples:
        lines.append("  samples: " + " | ".join(samples))

    search = [d for d in decisions if d.get("channel") == "search"]
    search_served = [d for d in search if d.get("count", 0) > 0]
    lines.append(f"search: {len(search)} queries, {rate(len(search_served), len(search)):.1f}% hit rate")
    return lines


def usage_lines(db_path):
    if not os.path.exists(db_path):
        return [f"usage: ERROR reading {db_path}: file not found (collection failed)"]
    try:
        con = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
        try:
            row = con.execute(
                "SELECT COALESCE(SUM(pushed),0), COALESCE(SUM(retrieved),0), COALESCE(SUM(confirmed_helpful),0) FROM usage"
            ).fetchone()
        finally:
            con.close()
    except sqlite3.Error as e:
        return [f"usage: ERROR reading {db_path}: {e}"]
    pushed, retrieved, confirmed = row
    return [f"usage totals: pushed={pushed} retrieved={retrieved} confirmed_helpful={confirmed}"]


def main(argv):
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--gate-decisions")
    p.add_argument("--usage-db")
    p.add_argument("--hours", type=float, default=24.0)
    p.add_argument("--now")
    args = p.parse_args(argv)

    now = parse_ts(args.now) if args.now else datetime.now(timezone.utc)
    cutoff = now - timedelta(hours=args.hours)

    out = []
    if args.gate_decisions:
        decisions = load_decisions(args.gate_decisions, cutoff)
        if decisions is None:
            out.append(f"push: ERROR reading {args.gate_decisions}: file not found (collection failed)")
            out.append(f"search: ERROR reading {args.gate_decisions}: file not found (collection failed)")
        else:
            out.extend(push_search_lines(decisions))
    if args.usage_db:
        out.extend(usage_lines(args.usage_db))

    print("\n".join(out))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
