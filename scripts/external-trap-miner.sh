#!/usr/bin/env bash
# external-trap-miner — LICENSE-GATED external-repo fix-commit miner (#0133).
#
# Legal boundary: reads ONLY commit messages + unified diffs from cloned repos.
# It never fetches issue/PR discussion prose (structurally impossible via this
# path). Permissive SPDX allowlist before clone: MIT, Apache-2.0, BSD-2-Clause,
# BSD-3-Clause, ISC, 0BSD, Unlicense, WTFPL — commits and tests carry the repo
# license (docs/WEB_SOURCES.md row 16). Everything produced is a retro-queue
# entry only; this script does not drain, judge, or write to the corpus.
set -uo pipefail

SEEDFILE="${1:?usage: external-trap-miner.sh <seedfile> [per_repo_limit]}"
LIMIT="${2:-20}"
QUEUE="${QUEUE:-/home/ori/twiceshy-retro-queue}"
SEEN="${EXTERNAL_MINER_SEEN:-/home/ori/.config/twiceshy/external-miner-seen}"
MAXDIFF="${TWICESHY_GIT_MINER_MAXDIFF:-3500}"
MINDIFF="${TWICESHY_GIT_MINER_MINDIFF:-40}"
SCAN="${TWICESHY_GIT_MINER_SCAN:-500}"

command -v jq >/dev/null 2>&1 || { echo "jq required" >&2; exit 1; }
[ -f "$SEEDFILE" ] || { echo "seedfile not found: $SEEDFILE" >&2; exit 1; }
mkdir -p "$QUEUE" "$(dirname "$SEEN")"
touch "$SEEN"

permissive_license() {
	case "$1" in
	MIT|Apache-2.0|BSD-2-Clause|BSD-3-Clause|ISC|0BSD|Unlicense|WTFPL) return 0 ;;
	*) return 1 ;;
	esac
}

parse_github_url() {
	local url=$1
	local _owner _repo
	url="${url%.git}"
	if [[ "$url" =~ ^https://github\.com/([^/]+)/([^/]+)$ ]]; then
		_owner="${BASH_REMATCH[1]}"
		_repo="${BASH_REMATCH[2]}"
		printf '%s\n' "$_owner" "$_repo"
		return 0
	fi
	return 1
}

default_license_resolver() {
	local url=$1 owner repo spdx
	if ! mapfile -t parts < <(parse_github_url "$url"); then
		echo "skip $url: unresolvable host" >&2
		return 1
	fi
	owner="${parts[0]}"; repo="${parts[1]}"
	local auth=()
	[ -n "${GITHUB_TOKEN:-}" ] && auth=(-H "Authorization: Bearer $GITHUB_TOKEN")
	spdx="$(curl -fsSL "${auth[@]}" "https://api.github.com/repos/${owner}/${repo}/license" \
		| jq -r '.license.spdx_id // empty' 2>/dev/null || true)"
	printf '%s\n' "$spdx"
}

resolve_license() {
	local url=$1
	if [ -n "${LICENSE_RESOLVER:-}" ]; then
		"$LICENSE_RESOLVER" "$url"
	else
		default_license_resolver "$url"
	fi
}

clone_repo() {
	local url=$1 dest=$2
	if [ -n "${CLONE_CMD:-}" ]; then
		"$CLONE_CMD" "$url" "$dest"
	else
		git clone --depth "$SCAN" "$url" "$dest"
	fi
}

mine_repo() {
	local url=$1 owner=$2 repo=$3 license=$4
	local tmpdir n=0
	tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/ext-trap-miner-XXXXXX")" || return 0

	if ! clone_repo "$url" "$tmpdir"; then
		echo "clone failed: $url" >&2
		rm -rf "$tmpdir"
		return 0
	fi

	mapfile -t shas < <(git -C "$tmpdir" log --no-merges --pretty='%H %s' -"$SCAN" \
		| grep -iE '\b(fix|bug|regression|revert|broke|broken|wrong|stale|flak|leak|race|deadlock|hang|crash|502|timeout|panic)\b' \
		| awk '{print $1}')

	for sha in "${shas[@]}"; do
		[ "$n" -ge "$LIMIT" ] && break
		seen_key="${owner}/${repo}:${sha}"
		grep -qF "$seen_key" "$SEEN" && continue
		subject="$(git -C "$tmpdir" log -1 --pretty='%s' "$sha")"
		body="$(git -C "$tmpdir" log -1 --pretty='%b' "$sha" | grep -ivE '^Co-Authored-By:')"
		diff="$(git -C "$tmpdir" show --no-color --format='' "$sha" 2>/dev/null | head -c "$MAXDIFF")"
		if [ "${#diff}" -lt "$MINDIFF" ]; then
			echo "$seen_key" >>"$SEEN"
			continue
		fi
		payload="$(printf 'A bug fix from the %s/%s repository (commit %s).\n\nProblem & fix, in the author'\''s words (commit message):\n%s\n%s\n\nThe change that fixed it (unified diff, may be truncated):\n%s\n' \
			"$owner" "$repo" "${sha:0:12}" "$subject" "$body" "$diff")"
		source_url="https://github.com/${owner}/${repo}/commit/${sha}"
		ts="$(date -u +%Y-%m-%dT%H%M%SZ)"
		tmp="$(mktemp "$QUEUE/.extmine-XXXXXX")" || continue
		if jq -nc \
			--arg sid "git-${owner}-${repo}-${sha:0:12}" \
			--arg author "git-history" \
			--arg reason "ext-fix-commit:${owner}/${repo}" \
			--arg t "$payload" \
			--arg at "$ts" \
			--arg src "$source_url" \
			--arg lic "$license" \
			'{session_id:$sid, author:$author, reason:$reason, transcript:$t, captured_at:$at, source_url:$src, source_license:$lic}' >"$tmp"; then
			mv -f "$tmp" "$QUEUE/${ts//[:]/}-git-${owner}-${repo}-${sha:0:12}.json"
			echo "$seen_key" >>"$SEEN"
			n=$((n + 1))
			echo "queued ${owner}/${repo} ${sha:0:12}: $subject"
		else
			rm -f "$tmp"
		fi
	done
	rm -rf "$tmpdir"
	echo "done: queued $n fix-commit(s) from ${owner}/${repo}"
}

total=0
while IFS= read -r line || [ -n "$line" ]; do
	line="${line%%#*}"
	line="$(printf '%s' "$line" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
	[ -z "$line" ] && continue
	url="${line%% *}"
	# ecosystem="${line#"$url"}" — operator metadata; selection only (#0133)
	[ -z "$url" ] && continue

	if ! mapfile -t parts < <(parse_github_url "$url"); then
		echo "skip $url: unresolvable host" >&2
		continue
	fi
	owner="${parts[0]}"; repo="${parts[1]}"

	license="$(resolve_license "$url" | head -n1 | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
	if [ -z "$license" ] || [ "$license" = "null" ]; then
		echo "skip $url: license ${license:-empty} not in allowlist" >&2
		continue
	fi
	if ! permissive_license "$license"; then
		echo "skip $url: license $license not in allowlist" >&2
		continue
	fi

	mine_repo "$url" "$owner" "$repo" "$license"
	total=$((total + 1))
done <"$SEEDFILE"

echo "processed $total allowlisted repo(s) from $SEEDFILE"
