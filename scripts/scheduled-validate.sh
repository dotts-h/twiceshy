#!/usr/bin/env bash
# scheduled-validate.sh — the twiceshy nightly validation driver (issue 0043,
# ADR-0013 §A1 + §2: the PR/soak/veto window, the headline human-oversight net).
#
# Sibling of scheduled-import.sh. One nightly run:
#   1. (§7) short-circuits immediately if TWICESHY_PAUSE is set — BEFORE any mutation.
#   2. MERGE-DUE: auto-merges any prior `validate/*` PR whose soak window has
#      elapsed and that is still open + green (an operator CLOSING the PR during
#      the soak vetoes the batch — a closed PR is never in the open set, so it is
#      never merged). This realizes the cooldown without a multi-day-long service.
#   3. VALIDATE: on a dedicated branch, intake queued outcome reports, then run
#      `promote` and `adapt` (judge-gated), batch the WHOLE night into ONE commit
#      (the run id — the rollback boundary), and open ONE PR (= the held queue).
#      The committed run manifests under runs/ are the artifact the daily audit
#      (#0044) reads. An anomaly (promote/adapt exit 3, #0037) is NOT auto-merged:
#      it is held + alerted for human review.
#
# Promote/adapt write only judge-approved transitions; the PR is the trust
# boundary (ADR-0001 §6) — nothing reaches main without the soak + a green CI +
# no operator veto.
#
# Env knobs:
#   TWICESHY_REPO          DEDICATED validate clone (default /home/ori/twiceshy-validate).
#                          MUST NOT be a working checkout — this script `git reset --hard`s.
#   TWICESHY_JUDGE_URL     diverse-model judge endpoint (required — never bypassed).
#   TWICESHY_JUDGE_MODEL   judge model id (default gpt-oss:20b).
#   TWICESHY_DRAFTER_MODEL drafter family the judge must differ from (default qwen2.5-coder).
#   TWICESHY_REPORT_QUEUE  report intake queue (#0042); empty skips intake.
#   TWICESHY_SOAK_SECONDS  veto-window cooldown before a batch PR auto-merges
#                          (default 172800 = 48h, ADR-0013 §2). A later nightly run
#                          does the merge, so the service never sleeps for the soak.
#   TWICESHY_AUTOMERGE     1 = merge due PRs after the soak (default), 0 = leave open.
#   TWICESHY_PAUSE         emergency stop — any truthy value skips the whole run.
#   TWICESHY_ALERT_URL     ntfy topic for anomaly alerts (passed to promote/adapt, #0038).
#   NTFY_URL               ntfy topic for run notifications (optional).
#   TWICESHY_VALIDATE_DRYRUN  1 = run the local path (build/intake/promote/adapt/commit)
#                          but do NOT push, open a PR, or merge — for safe verification.
#   GO                     go toolchain (default /usr/local/go/bin/go).
set -euo pipefail

REPO="${TWICESHY_REPO:-/home/ori/twiceshy-validate}"
GO="${GO:-/usr/local/go/bin/go}"
JUDGE_URL="${TWICESHY_JUDGE_URL:-}"
JUDGE_MODEL="${TWICESHY_JUDGE_MODEL:-gpt-oss:20b}"
DRAFTER_MODEL="${TWICESHY_DRAFTER_MODEL:-qwen2.5-coder}"
VOTES="${TWICESHY_VOTES:-3}"
# Throughput cap + anomaly backstop (#0084). MAX_PROMOTIONS>0 makes a run stop
# CLEANLY at the cap (a mergeable batch; re-run to continue) instead of the
# count-anomaly halting every full batch. 0 = off (legacy: MAX_ACTIONS is then the
# only ceiling). When a cap is set the count-anomaly is moot (the cap governs), so
# raise MAX_PROMOTIONS only once the hold-cooldown is deployed, or each capped run
# still re-judges the held backlog.
MAXPROMOTIONS="${TWICESHY_MAX_PROMOTIONS:-0}"
MAXACTIONS="${TWICESHY_MAX_ACTIONS:-25}"
# Hold cooldown (#0084): a panel-declined record is not re-judged again until this
# window elapses, so the held backlog stops re-judging itself every run. 0 = off.
HOLDCOOLDOWN="${TWICESHY_HOLD_COOLDOWN:-168h}"
QUEUE="${TWICESHY_REPORT_QUEUE:-}"
SOAK="${TWICESHY_SOAK_SECONDS:-172800}"
AUTOMERGE="${TWICESHY_AUTOMERGE:-1}"
DRYRUN="${TWICESHY_VALIDATE_DRYRUN:-0}"
NTFY_URL="${NTFY_URL:-}"
NTFY_TOKEN="${NTFY_TOKEN:-}"
# Forge repo + prebuilt binary (see scheduled-import.sh). Default = the engine repo +
# build-from-source; the decoupled deployment sets TWICESHY_FORGEJO_REPO=claude/twiceshy-corpus
# and TWICESHY_BIN=<prebuilt> so this runs against a data-only corpus clone (ADR-0021).
FORGEJO_REPO="${TWICESHY_FORGEJO_REPO:-claude/twiceshy}"
# The corpus repo has exactly ONE CI workflow (the engine repo has three), so
# forgejo-ci-merge's default wait-for-3-terminal-runs gate would never fire
# there and every PR would time out unmerged (issue 0105 pile-up). Derive the
# gate from the repo; an explicit FORGEJO_CI_MIN_RUNS in the env still wins.
case "$FORGEJO_REPO" in */twiceshy-corpus) export FORGEJO_CI_MIN_RUNS="${FORGEJO_CI_MIN_RUNS:-1}";; esac
BIN="${TWICESHY_BIN:-}"
API="http://192.168.50.244:3030/api/v1/repos/${FORGEJO_REPO}"

notify() {
	[ -n "$NTFY_URL" ] || return 0
	# ntfy.radulescu.app is deny-all + topic-scoped: without the Bearer token (and a
	# topic in NTFY_URL) the POST 403s and the alert is silently lost.
	curl -fsS ${NTFY_TOKEN:+-H "Authorization: Bearer $NTFY_TOKEN"} -d "$1" "$NTFY_URL" >/dev/null 2>&1 || true
}

# §7 emergency stop: short-circuit BEFORE any mutation (no clone reset, no build).
case "${TWICESHY_PAUSE:-}" in
1 | true | TRUE | yes | on)
	echo "validate: TWICESHY_PAUSE set — paused, no run"
	notify "twiceshy validate: PAUSED (TWICESHY_PAUSE), no run"
	exit 0
	;;
esac

[ -n "$JUDGE_URL" ] || {
	echo "TWICESHY_JUDGE_URL must be set — auto-validation needs a diverse-model judge" >&2
	exit 1
}

cd "$REPO"
git fetch origin -q
git checkout main -q
git reset --hard origin/main -q
git fetch origin main -q || true
BASE_ARGS=()
if git rev-parse --verify -q origin/main >/dev/null; then
	BASE_ARGS=(-base origin/main)
fi
# Sweep untracked stragglers from a prior crashed run (a missing pathspec is a
# no-op, not an error) so the next batch never commits a stale manifest/record.
git clean -fdq -- experience/ runs/

tok="$(git config --get remote.origin.url | sed -E 's#.*//[^:]+:([^@]+)@.*#\1#')"

# --- Phase 2: merge any prior validate batch whose soak has elapsed -----------
# A PR still OPEN past the soak (not vetoed-by-close) and green is merged. The
# merge is done by a LATER nightly run, so this service never holds for the soak.
merge_due() {
	[ "$AUTOMERGE" = "1" ] || return 0
	command -v forgejo-ci-merge >/dev/null || return 0
	local now due_prs pr head sha created age
	now="$(date -u +%s)"
	due_prs="$(curl -fsS "$API/pulls?state=open&limit=50" -H "Authorization: token $tok" 2>/dev/null || echo '[]')"
	# Each open validate/* PR: merge if (now - created_at) >= SOAK. An anomalous
	# batch (its PR body carries the ANOMALY marker) is NEVER auto-merged here even
	# after the soak — the merge happens in a LATER run that has no memory of the
	# original run's anomaly flag, so the PR body is the durable "held for review"
	# signal. The operator merges or closes it by hand.
	while read -r pr head sha created anom; do
		[ -n "$pr" ] || continue
		case "$head" in validate/*) ;; *) continue ;; esac
		if [ "$anom" = "true" ]; then
			echo "validate: PR #${pr} (${head}) flagged ANOMALY — held for human review, not auto-merged"
			continue
		fi
		age=$((now - created))
		if [ "$age" -ge "$SOAK" ]; then
			if forgejo-ci-merge "$FORGEJO_REPO" "$pr" "$sha" "$REPO"; then
				notify "twiceshy validate: PR #${pr} (${head}) merged after ${age}s soak"
			else
				notify "twiceshy validate: PR #${pr} (${head}) merge failed after soak"
			fi
		fi
	done < <(printf '%s' "$due_prs" | jq -r '.[] | "\(.number) \(.head.ref) \(.head.sha) \(.created_at | fromdateiso8601) \((.body // "") | test("ANOMALY"))"')
}
merge_due

# --- Phase 3: run tonight's validation on a fresh branch ----------------------
# Resolve the engine binary: a PATH-installed prebuilt (decoupled corpus — no source
# in $REPO) or a build from this clone (legacy engine-repo deployment).
if [ -n "$BIN" ]; then
	# shellcheck source=lib/ensure-engine-fresh.sh
	source "$(dirname "${BASH_SOURCE[0]}")/lib/ensure-engine-fresh.sh"
	ensure_engine_fresh
	bin="$BIN"
else
	bindir="$(mktemp -d)"
	trap 'rm -rf "$bindir"' EXIT # don't leak the build dir in /tmp across nightly runs
	bin="$bindir/twiceshy"
	"$GO" build -o "$bin" ./cmd/twiceshy
fi

runid="run-$(date -u +%Y%m%dT%H%M%SZ)"
branch="validate/${runid}"
git checkout -b "$branch" -q

export TWICESHY_JUDGE_URL="$JUDGE_URL"
[ -n "${TWICESHY_ALERT_URL:-}" ] && export TWICESHY_ALERT_URL

abort() {
	echo "$1" >&2
	notify "twiceshy validate: $1"
	git checkout main -q
	git branch -D "$branch" -q
	exit "${2:-1}"
}

# Intake queued outcome reports so adapt has input (#0042). Best-effort.
if [ -n "$QUEUE" ]; then
	"$bin" intake-reports -corpus "$REPO" -queue "$QUEUE" "${BASE_ARGS[@]}" || notify "twiceshy validate: intake-reports failed (continuing)"
fi

mkdir -p "$REPO/runs"
anomaly=0

# promote (positive direction). Exit 3 = anomaly halt (#0037) — keep going to
# capture adapt too, but mark the batch for human review (no auto-merge).
set +e
"$bin" promote -json -corpus "$REPO" -judge-model "$JUDGE_MODEL" -drafter-model "$DRAFTER_MODEL" -votes "$VOTES" -max-promotions "$MAXPROMOTIONS" -max-actions "$MAXACTIONS" -hold-cooldown "$HOLDCOOLDOWN" >"$REPO/runs/${runid}-promote.json" 2>"$REPO/runs/${runid}-promote.err"
pc=$?
set -e
case "$pc" in
0) ;;
3) anomaly=1 ;;
*) abort "promote failed (exit $pc): $(tr -d '\n' <"$REPO/runs/${runid}-promote.err" | tail -c 300)" "$pc" ;;
esac

# adapt (negative direction).
set +e
"$bin" adapt -json -corpus "$REPO" -judge-model "$JUDGE_MODEL" -drafter-model "$DRAFTER_MODEL" -max-promotions "$MAXPROMOTIONS" -max-actions "$MAXACTIONS" >"$REPO/runs/${runid}-adapt.json" 2>"$REPO/runs/${runid}-adapt.err"
ac=$?
set -e
case "$ac" in
0) ;;
3) anomaly=1 ;;
*) abort "adapt failed (exit $ac): $(tr -d '\n' <"$REPO/runs/${runid}-adapt.err" | tail -c 300)" "$ac" ;;
esac
rm -f "$REPO/runs/${runid}-promote.err" "$REPO/runs/${runid}-adapt.err"

# Batch the whole night into ONE commit (status, not diff — new files are untracked).
status="$(git status --porcelain)"
if [ -z "$status" ]; then
	echo "validate: nothing changed this run (${runid})"
	git checkout main -q
	git branch -D "$branch" -q
	exit 0
fi
git add -A
git commit -q \
	-m "validate(${runid}): nightly promote/adapt batch" \
	-m "Autonomous validation run (issue 0043, ADR-0013 §2). One commit = the rollback boundary; this PR is the held queue — CLOSE it to veto the batch. anomaly=${anomaly}." \
	-m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
sha="$(git rev-parse HEAD)"

if [ "$DRYRUN" = "1" ]; then
	echo "[dry-run] would push ${branch} and open a veto-window PR (anomaly=${anomaly}, sha=${sha})"
	git checkout main -q
	git branch -D "$branch" -q # leave no local branch behind on repeated dry-runs
	exit 0
fi

git push -q -u origin "$branch"

body="Autonomous nightly validation batch \`${runid}\`. Soak ${SOAK}s before auto-merge; **CLOSE this PR to veto** the whole batch. Run manifests are under \`runs/\`."
if [ "$anomaly" = "1" ]; then
	body="⚠️ **ANOMALY** detected this run (a guardrail tripped) — review before the soak elapses; it will NOT auto-merge. ${body}"
fi
pr="$(jq -nc --arg t "validate(${runid}): nightly promote/adapt batch" --arg b "$body" --arg h "$branch" \
	'{title:$t,body:$b,head:$h,base:"main"}' |
	curl -fsS -X POST "$API/pulls" -H "Authorization: token $tok" -H "Content-Type: application/json" -d @- |
	jq -r '.number')"

git checkout main -q
notify "twiceshy validate: opened PR #${pr} (${runid}); soak ${SOAK}s, close to veto$([ "$anomaly" = 1 ] && echo ' — ANOMALY, held for review')"
echo "done: ${runid}, PR #${pr}, anomaly=${anomaly}"
