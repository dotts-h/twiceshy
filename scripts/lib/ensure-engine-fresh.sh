#!/usr/bin/env bash
# ensure-engine-fresh.sh — keep scheduled jobs aligned with MERGED engine main.
#
# A scheduled job's wrapper script ships via the repo (current the instant a commit
# merges), but the deployed binary is built separately and can lag — so the wrapper
# can pass a flag the stale binary rejects, and the job dies silently (exp-2840,
# #0096). This helper makes the deployed binary track `origin/main`: if it is behind,
# rebuild from a clean main checkout; if it is stale but the checkout is NOT on clean
# main (e.g. a dev feature branch), refuse LOUDLY rather than build unmerged code or
# silently run a stale binary.
set -uo pipefail

ENGINE_REPO="${ENGINE_REPO:-/home/ori/twiceshy}"
BIN="${BIN:-$HOME/.local/bin/twiceshy}"
ENGINE_BUILD_MARKER="${ENGINE_BUILD_MARKER:-$HOME/.local/state/twiceshy-built-commit}"

# --- seams (overridable in tests) -------------------------------------------------

# engine_target_commit: the commit the deployed binary SHOULD match — MERGED main,
# not the local working branch. Best-effort fetch, then resolve the cached ref so a
# transient forge blip falls back to last-known origin/main instead of blocking.
engine_target_commit() {
	git -C "$ENGINE_REPO" fetch -q origin main 2>/dev/null || true
	git -C "$ENGINE_REPO" rev-parse origin/main 2>/dev/null
}

# engine_build_ready: is the working checkout safe to build origin/main from? Only
# when it is ON main and CLEAN — never build a feature branch or a dirty tree.
engine_build_ready() {
	[ "$(git -C "$ENGINE_REPO" symbolic-ref --short -q HEAD 2>/dev/null)" = "main" ] \
		&& git -C "$ENGINE_REPO" diff --quiet 2>/dev/null \
		&& git -C "$ENGINE_REPO" diff --cached --quiet 2>/dev/null
}

# build_engine: fast-forward main to origin/main (safe — no merge commits) and build.
build_engine() { (cd "$ENGINE_REPO" && git merge --ff-only -q origin/main && go build -o "$BIN" ./cmd/twiceshy); }

log() { logger -t ensure-engine-fresh "$*" 2>/dev/null || true; echo "$*"; }
alert() {
	local msg="$*" url="${TWICESHY_ALERT_URL:-${NTFY_URL:-}}"
	log "ALERT: $msg"
	[ -n "$url" ] || return 0
	curl -fsS -m 10 ${NTFY_TOKEN:+-H "Authorization: Bearer $NTFY_TOKEN"} \
		-d "ensure-engine-fresh: $msg" "$url" >/dev/null 2>&1 || true
}

ensure_engine_fresh() {
	local want have
	# Escape hatch: skip the preflight entirely (hermetic unit tests of the scheduled
	# scripts; or an operator who deploys the engine out-of-band). Leave UNSET in
	# production so the staleness guard actually runs.
	[ -n "${TWICESHY_SKIP_ENGINE_FRESH:-}" ] && return 0
	want="$(engine_target_commit)" || true
	[ -n "$want" ] || { alert "cannot resolve engine origin/main at $ENGINE_REPO"; return 1; }
	have="$(cat "$ENGINE_BUILD_MARKER" 2>/dev/null || true)"

	if [ "$want" = "$have" ] && [ -e "$BIN" ]; then
		return 0
	fi
	# Stale (or missing) binary. Only self-heal from a clean main checkout; otherwise
	# fail LOUD — never build unmerged/dirty code, never silently run a stale binary.
	if ! engine_build_ready; then
		alert "engine binary stale vs origin/main ($have→$want) but checkout not on clean main — deploy from main manually"
		return 1
	fi
	if ! build_engine; then
		alert "engine rebuild FAILED $have→$want"
		return 1
	fi
	mkdir -p "$(dirname "$ENGINE_BUILD_MARKER")"
	if ! printf '%s\n' "$want" > "$ENGINE_BUILD_MARKER"; then
		alert "engine rebuilt but build marker update FAILED: $ENGINE_BUILD_MARKER"
		return 1
	fi
	log "rebuilt engine $have→$want"
	alert "rebuilt engine $have→$want"
}

if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
	ensure_engine_fresh "$@"
fi
