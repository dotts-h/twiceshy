#!/usr/bin/env bash
# shellcheck disable=SC2317  # stub fns (promoted_counts/eligible_quarantine_count/now/notify) are invoked indirectly by the sourced script
# Tests for promotions-liveness-alarm.sh — sources the script (source-guard stops
# main) and stubs the seams. The alarm is the missing net for #0122: the judge froze
# for ~5 days promoting ZERO while every run stayed green (anomaly=0). It fires when
# the last K promote manifests ALL promoted 0 AND the corpus still holds quarantined
# records that are eligible for judging (holds/cooldown excluded — otherwise an
# idle-but-healthy system false-positives). Run: bash promotions-liveness-alarm.test.sh
set -uo pipefail
cd "$(dirname "$0")" || exit 1

PASS=0
FAIL=0
ok()  { PASS=$((PASS + 1)); printf 'PASS %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL %s\n' "$1"; }
check() { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (got [$2] want [$3])"; fi; }
contains() { case "$1" in *"$2"*) return 0 ;; *) return 1 ;; esac; }

export TWICESHY_LIVENESS_K=3 TWICESHY_LIVENESS_COOLDOWN=600
STATE_FILE="$(mktemp)"; rm -f "$STATE_FILE"
export TWICESHY_LIVENESS_STATE="$STATE_FILE"
# shellcheck source=/dev/null
source ./promotions-liveness-alarm.sh
set +e

# ---- stubbed seams (the default suite drives logic through these) ----------------
COUNTS=""          # one promoted-count per line (newest first), up to K lines
ELIGIBLE=0         # eligible (non-cooldown) quarantined record count
NOW=1000000        # fixed clock (seconds)
ALERTS=""
reset() { COUNTS=""; ELIGIBLE=0; ALERTS=""; rm -f "$STATE_FILE"; }

promoted_counts()          { printf '%s' "$COUNTS"; }
eligible_quarantine_count() { echo "$ELIGIBLE"; }
now()                      { echo "$NOW"; }
notify()                   { ALERTS="${ALERTS}|$1"; }

# ---- fewer than K manifests: not enough history, never alarm --------------------
reset; COUNTS="0
0"; ELIGIBLE=99
main; rc=$?
check "history < K returns 0" "$rc" "0"
if [ -z "$ALERTS" ]; then ok "history < K => no alarm"; else bad "history < K must not alarm: $ALERTS"; fi

# ---- K manifests, one promoted > 0: the loop is alive, no alarm -----------------
reset; COUNTS="0
2
0"; ELIGIBLE=99
main; rc=$?
check "a productive run returns 0" "$rc" "0"
if [ -z "$ALERTS" ]; then ok "any promoted>0 in window => no alarm"; else bad "productive window must not alarm: $ALERTS"; fi

# ---- K all-zero but nothing eligible (idle/all-in-cooldown): healthy quiet ------
reset; COUNTS="0
0
0"; ELIGIBLE=0
main; rc=$?
check "idle (0 eligible) returns 0" "$rc" "0"
if [ -z "$ALERTS" ]; then ok "promoted=0 but 0 eligible => no alarm (idle-healthy)"; else bad "idle-healthy must not alarm: $ALERTS"; fi

# ---- the freeze: K all-zero AND eligible quarantine backlog -> ALARM -------------
reset; COUNTS="0
0
0"; ELIGIBLE=1873
main; rc=$?
check "the freeze returns non-zero" "$rc" "1"
if contains "$ALERTS" "1873"; then ok "freeze alarm reports the eligible count"; else bad "freeze must alarm with the count: $ALERTS"; fi
if contains "$ALERTS" "promoted 0"; then ok "freeze alarm names the promoted-0 symptom"; else bad "alarm must name the symptom: $ALERTS"; fi

# ---- cooldown: a still-frozen tick within COOLDOWN does not re-alarm -------------
reset; COUNTS="0
0
0"; ELIGIBLE=1873
main >/dev/null; ALERTS=""            # first tick alarms + stamps the state file
NOW=$((NOW + 60))                     # 60s later, still frozen, still < COOLDOWN
main; rc=$?
if [ -z "$ALERTS" ]; then ok "within cooldown => no repeat storm"; else bad "cooldown must suppress the repeat: $ALERTS"; fi
NOW=$((NOW + 601))                    # past the cooldown: remind once more
main
if contains "$ALERTS" "1873"; then ok "past cooldown => re-alarms (reminds, never storms)"; else bad "past cooldown must re-alarm: $ALERTS"; fi

# ==================================================================================
# Integration: exercise the REAL data-gathering seams against a fixture repo. Because
# bash's `unset -f` on a shadowed function does NOT restore the original definition,
# the real seams are reached by re-sourcing the script in a CLEAN namespace via a
# child `bash -c` (which does not inherit this shell's stub functions), rather than
# by unsetting the stubs here.
# ==================================================================================
REPO="$(mktemp -d)"; mkdir -p "$REPO/runs" "$REPO/experience/2026"
mkfr() { # id status -> a minimal record file
  printf -- '---\nschema_version: 1\nid: %s\nstatus: %s\n---\nbody\n' "$1" "$2" > "$REPO/experience/2026/$1.md"
}
mkmanifest() { # runid promoted -> a promote manifest
  printf '{"run_id":"%s","stage":"promote","anomaly":false,"counts":{"held":0,"ineligible":0,"promoted":%s},"actions":[]}\n' "$1" "$2" > "$REPO/runs/$1-promote.json"
}
# three quarantined records; one of them is in the hold ledger (cooldown) => 2 eligible
mkfr exp-0001 quarantined; mkfr exp-0002 quarantined; mkfr exp-0003 quarantined
mkfr exp-0100 validated
printf '{"exp-0003":"2026-07-08T00:00:00Z"}\n' > "$REPO/runs/promote.holds.json"
mkmanifest run-20260709T010000Z 0
mkmanifest run-20260709T020000Z 0
mkmanifest run-20260709T030000Z 0

# run <fn-or-main> in a fresh shell with only the real script sourced (no stubs).
run_real() { TWICESHY_REPO="$REPO" TWICESHY_LIVENESS_STATE="$STATE_FILE" \
  bash -c 'cd "$1"; source ./promotions-liveness-alarm.sh; shift; "$@"' _ "$PWD" "$@"; }

check "real eligible_quarantine_count excludes the hold ledger" "$(run_real eligible_quarantine_count)" "2"
check "real promoted_counts reads 3 zero-promote manifests" "$(run_real promoted_counts | tr '\n' ',')" "0,0,0,"
rm -f "$STATE_FILE"
# Execute the real script as a subprocess against the frozen fixture; capture its ntfy
# body by pointing notify() at a log-only ALERT_URL is overkill — assert on exit code
# (1 = frozen) and on the logger-less notify text via a captured NTFY sink.
sink="$(mktemp)"
TWICESHY_REPO="$REPO" TWICESHY_LIVENESS_STATE="$STATE_FILE" NOWFILE="$sink" \
  bash -c 'cd "$1"; source ./promotions-liveness-alarm.sh
           notify() { printf "%s" "$1" > "$NOWFILE"; }
           main' _ "$PWD"
rc=$?
check "real seams: frozen fixture alarms" "$rc" "1"
if contains "$(cat "$sink")" "2 quarantined"; then ok "real alarm reports 2 eligible"; else bad "real alarm count wrong: $(cat "$sink")"; fi
rm -rf "$REPO" "$sink"

# ---- summary ---------------------------------------------------------------------
printf '\n----\nPASS=%d FAIL=%d\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
