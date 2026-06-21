#!/usr/bin/env bash
# continuous-pump.sh — supervised full-power ingest+validate loop (2026-06-21).
#
# The nightly timers fire once a day; this loops the SAME import + validate
# systemd services on a short interval so the corpus ingests new advisories and
# validates them continuously over a bounded window. It deliberately reuses the
# existing units (not the scripts directly) so every env/PATH/token detail stays
# exactly as the proven nightly path — each `systemctl start` of a Type=oneshot
# blocks until that run finishes, so cycles never overlap.
#
# Landing cadence is governed by validate.env (TWICESHY_AUTOMERGE / _SOAK_SECONDS);
# for the supervised run those are set to auto-merge after a short veto window.
#
# Run (survives the launching shell):
#   sudo systemd-run --unit=twiceshy-pump --collect \
#     -E PUMP_HOURS=9 /home/ori/twiceshy/scripts/continuous-pump.sh
# Watch:  journalctl -u twiceshy-pump -f
# Stop:   sudo systemctl stop twiceshy-pump
set -uo pipefail
HOURS="${PUMP_HOURS:-9}"
INTERVAL="${PUMP_INTERVAL:-1500}" # 25 min between cycle starts
END=$(( $(date +%s) + HOURS * 3600 ))
i=0
echo "twiceshy-pump: START $(date -u +%FT%TZ) — ${HOURS}h window, ${INTERVAL}s interval"
while [ "$(date +%s)" -lt "$END" ]; do
	i=$((i + 1))
	echo "=== cycle $i: import   $(date -u +%FT%TZ) ==="
	systemctl start twiceshy-import.service || echo "cycle $i: import service failed (continuing)"
	echo "=== cycle $i: validate $(date -u +%FT%TZ) ==="
	systemctl start twiceshy-validate.service || echo "cycle $i: validate service failed (continuing)"
	rem=$(( END - $(date +%s) ))
	[ "$rem" -le 0 ] && break
	nap=$(( rem < INTERVAL ? rem : INTERVAL ))
	echo "=== cycle $i done $(date -u +%FT%TZ); sleeping ${nap}s (${rem}s left) ==="
	sleep "$nap"
done
echo "twiceshy-pump: COMPLETE after $i cycles $(date -u +%FT%TZ)"
