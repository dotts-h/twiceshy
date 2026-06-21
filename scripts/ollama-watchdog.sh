#!/usr/bin/env bash
# ollama-watchdog.sh — keep the Ollama VM (the gpt-oss judge upstream) up "always".
# onboot=1 covers a host reboot; this covers a runtime crash/hang of VM 101 by
# probing the API and restarting the VM via the Proxmox host if it's unreachable.
# Idempotent: `qm start` on an already-running VM is a quick no-op. Runs as ori,
# which holds the brain key authorized on the Proxmox host root.
set -uo pipefail
OLLAMA_URL="${OLLAMA_URL:-http://192.168.50.150:11434/api/version}"
PVE="${PVE_HOST:-root@192.168.50.2}"
VMID="${OLLAMA_VMID:-101}"

if curl -fsS --max-time 8 "$OLLAMA_URL" >/dev/null 2>&1; then
	exit 0 # healthy
fi
logger -t ollama-watchdog "ollama unreachable at $OLLAMA_URL — starting VM $VMID"
ssh -o StrictHostKeyChecking=no -o ConnectTimeout=6 "$PVE" "qm start $VMID" 2>&1 | logger -t ollama-watchdog || true
