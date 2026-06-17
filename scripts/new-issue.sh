#!/usr/bin/env bash
# new-issue.sh "<title>" [--epic] [--group <epic-id>] [--severity <low|medium|high|critical>] [--depends id,id]
# Creates docs/issues/NNNN-title.md from the canonical format (docs/issues/TEMPLATE.md),
# appends its row to docs/issues/INDEX.md, and prints the file path.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

title="${1:-}"; shift || true
group=""; sev="medium"; deps=""; epic=0
while [ $# -gt 0 ]; do case "$1" in
  --epic) epic=1; shift;;
  --group) group="$2"; shift 2;;
  --severity) sev="$2"; shift 2;;
  --depends) deps="$2"; shift 2;;
  *) shift;;
esac; done
[ -n "$title" ] || { echo 'usage: new-issue.sh "<title>" [--epic] [--group id] [--severity s] [--depends id,id]' >&2; exit 2; }
if [ "$epic" -eq 1 ]; then
  case "$title" in Epic:*) ;; *) title="Epic: $title";; esac
fi

# Normalize deps ("1, 2") to a zero-padded YAML inline list ([0001, 0002]).
dep_yaml="[]"
if [ -n "$deps" ]; then
  dep_yaml="[$(printf '%s\n' "$deps" | grep -oE '[0-9]+' \
    | while read -r d; do printf '%04d, ' "$((10#$d))"; done | sed 's/, $//')]"
fi

dir="docs/issues"; mkdir -p "$dir/assets"
last=0
for f in "$dir"/[0-9][0-9][0-9][0-9]-*.md; do
  [ -e "$f" ] || continue
  n=$(basename "$f" | cut -c1-4); n=$((10#$n))
  [ "$n" -gt "$last" ] && last=$n
done
id=$(printf "%04d" $((last+1)))
slug=$(printf '%s' "$title" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//')
path="$dir/${id}-${slug}.md"

cat > "$path" <<EOF
---
id: ${id}
title: ${title}
status: open
severity: ${sev}
group: ${group}
depends_on: ${dep_yaml}
github:
forgejo:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

## Repro
1.
Expected:
Actual:

## Evidence

## Notes
EOF

# Append the row to the INDEX's Issues table (it is the last table in the file).
idx="$dir/INDEX.md"
if [ -f "$idx" ]; then
  printf '| [%s](%s) | %s | open | %s | %s | |\n' \
    "$id" "$(basename "$path")" "$title" "$sev" "${group:-—}" >> "$idx"
else
  echo "WARN: $idx missing — row not recorded; create the index (issues recipe) and add it" >&2
fi

echo "$path"
