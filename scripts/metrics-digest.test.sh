#!/usr/bin/env bash
# Tests for metrics-digest.sh + metrics-digest-aggregate.py (#0116). Two layers,
# hooks/twiceshy-error-pull.test.sh style:
#   1. The pure aggregation core (metrics-digest-aggregate.py) driven directly
#      against fixture JSONL/sqlite files — the numeric logic, hermetic, no ssh/
#      docker/journalctl.
#   2. metrics-digest.sh sourced with its collection/journal/notify seams
#      stubbed (mirrors twiceshy-growth-watchdog.test.sh) — the composition +
#      "never silent again" partial-digest behavior on a collection failure.
# shellcheck disable=SC2317  # seam stubs are invoked indirectly by the sourced script
set -uo pipefail
cd "$(dirname "$0")" || exit 1

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

command -v python3 >/dev/null 2>&1 || { echo "SKIP: python3 not installed"; exit 0; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# ---- layer 1: metrics-digest-aggregate.py ----------------------------------------

GD="$TMP/gate-decisions.jsonl"
cat >"$GD" <<'EOF'
{"ts":"2026-07-01T10:00:00Z","channel":"push","trigger":"prompt","count":1,"served":[{"id":"exp-0001","score":1}],"query_text":"how to fix foo bar baz"}
{"ts":"2026-07-01T10:05:00Z","channel":"push","trigger":"prompt","count":0,"served":[]}
{"ts":"2026-07-01T10:10:00Z","channel":"push","trigger":"error","count":1,"served":[{"id":"exp-0001","score":1}]}
{"ts":"2026-07-01T10:20:00Z","channel":"search","count":1,"served":[{"id":"exp-0002","score":1}]}
{"ts":"2026-07-01T10:25:00Z","channel":"search","count":0,"served":[]}
{"ts":"2025-01-01T00:00:00Z","channel":"push","trigger":"prompt","count":1,"served":[{"id":"exp-9999","score":1}]}
EOF

out="$(python3 metrics-digest-aggregate.py --gate-decisions "$GD" --hours 24 --now "2026-07-01T12:00:00Z")"
if contains "$out" "push: 3 queries, 2 served (66.7%)"; then ok "push total + served rate"; else bad "push total + served rate: $out"; fi
if contains "$out" "trigger=prompt: 2 queries, 1 served (50.0%)"; then ok "trigger=prompt split"; else bad "trigger=prompt split: $out"; fi
if contains "$out" "trigger=error: 1 queries, 1 served (100.0%)"; then ok "trigger=error split"; else bad "trigger=error split: $out"; fi
if contains "$out" "top served: exp-0001x2"; then ok "top served ids"; else bad "top served ids: $out"; fi
if contains "$out" "samples: how to fix foo bar baz"; then ok "query_text sample"; else bad "query_text sample: $out"; fi
if contains "$out" "search: 2 queries, 50.0% hit rate"; then ok "search hit rate"; else bad "search hit rate: $out"; fi
if contains "$out" "exp-9999"; then bad "a decision outside the window must not be counted: $out"; else ok "window excludes old decisions"; fi

# A collection failure (missing path) marks the section, not silently zero.
out="$(python3 metrics-digest-aggregate.py --gate-decisions "$TMP/missing.jsonl" --hours 24)"
if contains "$out" "push: ERROR" && contains "$out" "search: ERROR"; then
	ok "missing gate-decisions file marks push+search ERROR"
else
	bad "missing gate-decisions file must mark ERROR, not silently zero: $out"
fi

# usage db aggregation.
DB="$TMP/usage.db"
python3 - "$DB" <<'PY'
import sqlite3, sys
con = sqlite3.connect(sys.argv[1])
con.execute("CREATE TABLE usage (record_id TEXT PRIMARY KEY, retrieved INTEGER NOT NULL DEFAULT 0, pushed INTEGER NOT NULL DEFAULT 0, confirmed_helpful INTEGER NOT NULL DEFAULT 0, last_hit TEXT)")
con.execute("INSERT INTO usage VALUES ('exp-0001', 5, 3, 2, NULL)")
con.execute("INSERT INTO usage VALUES ('exp-0002', 1, 1, 0, NULL)")
con.commit(); con.close()
PY
out="$(python3 metrics-digest-aggregate.py --usage-db "$DB")"
if contains "$out" "usage totals: pushed=4 retrieved=6 confirmed_helpful=2"; then ok "usage totals sum across records"; else bad "usage totals: $out"; fi

out="$(python3 metrics-digest-aggregate.py --usage-db "$TMP/missing.db")"
if contains "$out" "usage: ERROR"; then ok "missing usage db marks ERROR"; else bad "missing usage db must mark ERROR: $out"; fi

# ---- layer 2: metrics-digest.sh (seams stubbed) ----------------------------------

# Unlike layer 1 above (which pins "now" via aggregate.py's --now), main() below
# calls aggregate.py with no --now override, so it windows against the REAL
# wall clock — $GD's hardcoded 2026-07-01 timestamps age out of the 24h window
# as real time passes (#0131: this is exactly what broke "healthy digest missing
# push section" once "now" moved past 2026-07-02). Anchor this fixture's
# timestamps a couple of hours behind the actual current time instead, so the
# test stays green regardless of when it runs.
recent_ts() { date -u -d "-$1 minutes" +%Y-%m-%dT%H:%M:%SZ; }
GD_RECENT="$TMP/gate-decisions-recent.jsonl"
cat >"$GD_RECENT" <<EOF
{"ts":"$(recent_ts 120)","channel":"push","trigger":"prompt","count":1,"served":[{"id":"exp-0001","score":1}],"query_text":"how to fix foo bar baz"}
{"ts":"$(recent_ts 115)","channel":"push","trigger":"prompt","count":0,"served":[]}
{"ts":"$(recent_ts 110)","channel":"push","trigger":"error","count":1,"served":[{"id":"exp-0001","score":1}]}
{"ts":"$(recent_ts 100)","channel":"search","count":1,"served":[{"id":"exp-0002","score":1}]}
{"ts":"$(recent_ts 95)","channel":"search","count":0,"served":[]}
EOF

export TWICESHY_NTFY_ENV="$TMP/ntfy.env" # empty: no NTFY_URL -> notify is a log-only no-op
: >"$TWICESHY_NTFY_ENV"
export DIGEST_HOURS=24
# shellcheck source=/dev/null
source ./metrics-digest.sh
set +e

NOTIFIED=""
notify() { NOTIFIED="$1"; }

reset() { NOTIFIED=""; }

# Healthy tick: both sources fetch, journal seams return canned drain output.
reset
fetch_gate_decisions() { cp "$GD_RECENT" "$1"; }
fetch_usage_db() { cp "$DB" "$1"; }
journal_retro() { printf '  confirmed 2 helpful (from x)\n  skip y: unprocessable after 3 attempts (err)\n'; }
journal_validate() { printf 'done: run-1, PR #5, anomaly=0\n'; }
main >/dev/null
if contains "$NOTIFIED" "push: 3 queries"; then ok "healthy digest includes push section"; else bad "healthy digest missing push section: $NOTIFIED"; fi
if contains "$NOTIFIED" "usage totals: pushed=4"; then ok "healthy digest includes usage section"; else bad "healthy digest missing usage: $NOTIFIED"; fi
if contains "$NOTIFIED" "1 confirmed-helpful run(s), 0 confirmed-zero run(s), 1 unprocessable-after-retries"; then ok "retro section counts"; else bad "retro section: $NOTIFIED"; fi
if contains "$NOTIFIED" "1 run(s) logged, 0 anomaly-flagged"; then ok "validate section counts"; else bad "validate section: $NOTIFIED"; fi
if contains "$NOTIFIED" "n/a (not greppable"; then ok "validate section notes promoted/held n/a"; else bad "validate n/a note missing: $NOTIFIED"; fi
if contains "$NOTIFIED" "baseline: served rate was ~70%"; then ok "digest carries the pre-fix baseline"; else bad "baseline note missing: $NOTIFIED"; fi

# A collection failure must still post a PARTIAL digest with the failing section
# marked — never a silently dropped digest (exp-0746/#0072).
reset
fetch_gate_decisions() { return 1; }
fetch_usage_db() { cp "$DB" "$1"; }
main >/dev/null
if [ -z "$NOTIFIED" ]; then
	bad "a collection failure must still post a partial digest, not silence it"
else
	ok "collection failure still posts (non-empty digest)"
fi
if contains "$NOTIFIED" "push: ERROR"; then ok "failed gate-decisions collection marks push ERROR"; else bad "push ERROR marker missing: $NOTIFIED"; fi
if contains "$NOTIFIED" "usage totals: pushed=4"; then ok "the healthy usage section still reports despite the other failure"; else bad "usage section dropped alongside the unrelated failure: $NOTIFIED"; fi

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
