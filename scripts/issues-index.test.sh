#!/usr/bin/env bash
# Tests for docs/issues/INDEX.md reconciliation with issue-file frontmatter.
# It ensures every docs/issues/NNNN-*.md file has a matching row in docs/issues/INDEX.md,
# and that its status and group cells are reconciled.
# Run: bash scripts/issues-index.test.sh
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

PASS=0
FAIL=0

ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }

get_fm_val() {
  local file="$1"
  local key="$2"
  awk -v key="$key" '
    BEGIN { in_fm = 0; fm_count = 0; val = "" }
    /^---$/ {
      fm_count++
      if (fm_count == 1) { in_fm = 1 }
      else if (fm_count == 2) { in_fm = 0 }
      next
    }
    in_fm {
      if ($0 ~ "^" key ":") {
        v = $0
        sub("^" key ":[ \t]*", "", v)
        sub(/[ \t\r]*$/, "", v)
        sub(/^"/, "", v); sub(/"$/, "", v)
        sub(/^'\''/, "", v); sub(/'\''$/, "", v)
        val = v
      }
    }
    END { print val }
  ' "$file"
}

for filepath in docs/issues/[0-9][0-9][0-9][0-9]-*.md; do
  [ -f "$filepath" ] || continue
  filename=$(basename "$filepath")
  id="${filename%%-*}"

  fm_status=$(get_fm_val "$filepath" "status")
  fm_group=$(get_fm_val "$filepath" "group")

  # b. Require at least one row matching '^| [NNNN](' in docs/issues/INDEX.md
  if ! grep -q "^| \[$id\](" docs/issues/INDEX.md; then
    bad "ID $id: missing row in docs/issues/INDEX.md"
    continue
  fi

  # c & d. Reconcile matching rows
  while IFS= read -r line; do
    [ -n "$line" ] || continue

    # count pipes in this line
    num_pipes=$(echo "$line" | tr -cd '|' | wc -c)

    # extract status (4th field) and group (6th field)
    IFS="|" read -r status_cell group_cell < <(
      echo "$line" | awk -F'|' '{
        sub(/^[ \t\r\n]*/, "", $4); sub(/[ \t\r\n]*$/, "", $4);
        sub(/^[ \t\r\n]*/, "", $6); sub(/[ \t\r\n]*$/, "", $6);
        print $4 "|" $6
      }'
    )

    # c. check status
    if [ "$status_cell" != "$fm_status" ]; then
      bad "ID $id status mismatch: frontmatter is '$fm_status', index is '$status_cell'"
    else
      ok "ID $id status matches ($status_cell)"
    fi

    # Rows are either epics-table (5 pipes: id|title|status|children) or
    # issues-table (7 pipes: id|title|status|severity|group|links) shaped;
    # anything else is a malformed row whose cells can't be trusted.
    if [ "$num_pipes" -ne 5 ] && [ "$num_pipes" -ne 7 ]; then
      bad "ID $id row has unexpected column count ($num_pipes pipes)"
    fi

    # d. check group if >= 6 data columns (which means >= 7 pipes)
    if [ "$num_pipes" -ge 7 ]; then
      norm_group_cell="$group_cell"
      if [ "$norm_group_cell" = "—" ] || [ "$norm_group_cell" = "" ]; then
        norm_group_cell=""
      fi

      if [ "$norm_group_cell" != "$fm_group" ]; then
        bad "ID $id group mismatch: frontmatter is '$fm_group', index is '$group_cell'"
      else
        ok "ID $id group matches ($group_cell)"
      fi
    fi
  done < <(grep "^| \[$id\](" docs/issues/INDEX.md || true)
done

# Orphaned INDEX rows: every row's id must have a matching issue file.
while IFS= read -r row_id; do
  if ! ls "docs/issues/${row_id}"-*.md >/dev/null 2>&1; then
    bad "ID $row_id: INDEX row has no matching docs/issues/${row_id}-*.md file"
  else
    ok "ID $row_id row has a matching issue file"
  fi
done < <(grep -o '^| \[[0-9][0-9][0-9][0-9]\](' docs/issues/INDEX.md | grep -o '[0-9][0-9][0-9][0-9]' | sort -u)

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
