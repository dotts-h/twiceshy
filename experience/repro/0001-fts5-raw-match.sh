#!/usr/bin/env sh
# F2P repro for exp-0001: raw user input in FTS5 MATCH is query syntax.
#
# Fail-to-pass discipline (docs/SCHEMA.md, guard.repro):
#   - demonstrates the trap state: the raw identifier errors out;
#   - demonstrates the escape: the per-token double-quoted form matches;
#   - exit 0  = trap reproduced AND escape works (record is valid);
#   - exit 1  = the world changed (trap gone or escape broken) -> stale;
#   - exit 75 = environment cannot run the repro (skip, EX_TEMPFAIL).
set -u

command -v sqlite3 >/dev/null 2>&1 || { echo "SKIP: sqlite3 CLI not available"; exit 75; }
sqlite3 ":memory:" "CREATE VIRTUAL TABLE t USING fts5(body);" >/dev/null 2>&1 \
  || { echo "SKIP: sqlite3 built without FTS5"; exit 75; }

db="$(mktemp)" || exit 75
trap 'rm -f "$db"' EXIT

sqlite3 "$db" "CREATE VIRTUAL TABLE t USING fts5(body);
               INSERT INTO t VALUES ('driver error near modernc.org/sqlite call');" || exit 75

# Trap state: the raw identifier is FTS5 query syntax and must error.
if sqlite3 "$db" "SELECT body FROM t WHERE t MATCH 'modernc.org/sqlite';" >/dev/null 2>&1; then
  echo "NOT REPRODUCED: raw 'modernc.org/sqlite' was accepted by MATCH"
  exit 1
fi

# Escape: the double-quoted token must be a plain term and match the row.
out="$(sqlite3 "$db" "SELECT body FROM t WHERE t MATCH '\"modernc.org/sqlite\"';" 2>&1)" || {
  echo "ESCAPE BROKEN: quoted token errored: $out"
  exit 1
}
[ -n "$out" ] || { echo "ESCAPE BROKEN: quoted token matched nothing"; exit 1; }

echo "OK: raw input is rejected as syntax, quoted token matches"
exit 0
