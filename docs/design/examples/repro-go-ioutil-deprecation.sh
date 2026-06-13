#!/usr/bin/env bash
# Proof-of-concept fail-to-pass guard for exp-poc-0001 (Go io/ioutil
# deprecation). Demonstrates how a "codemod-class" record synthesizes an
# executable guard: it proves the trap is real (staticcheck SA1019 fires on the
# pre-fix code) and that the fix works (staticcheck is clean post-fix).
#
# Fail-to-pass discipline (docs/SCHEMA.md): exits non-zero in the trap/pre-fix
# state checks, zero when the full before->after holds, and 75 when the
# environment can't run it (tools missing) so the doctor skips rather than fails.
set -euo pipefail

command -v go >/dev/null 2>&1 || { echo "go not installed"; exit 75; }
command -v staticcheck >/dev/null 2>&1 || {
  echo "staticcheck not installed (go install honnef.co/go/tools/cmd/staticcheck@latest)"
  exit 75
}

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cd "$work"
go mod init reprotmp >/dev/null 2>&1

# --- BEFORE: the trap. Deprecated io/ioutil call. ---
cat > main.go <<'EOF'
package main

import (
	"io/ioutil"
	"log"
)

func main() {
	b, err := ioutil.ReadFile("/etc/hostname")
	if err != nil {
		log.Fatal(err)
	}
	_ = b
}
EOF

# The trap must reproduce: staticcheck SA1019 fires on the deprecated call.
if staticcheck ./... 2>&1 | grep -q "SA1019"; then
	echo "OK: trap reproduced (SA1019 on ioutil.ReadFile)"
else
	echo "FAIL: expected SA1019 on the pre-fix code, got none"
	exit 1
fi

# --- AFTER: apply the 1:1 rewrite (ioutil.ReadFile -> os.ReadFile). ---
cat > main.go <<'EOF'
package main

import (
	"log"
	"os"
)

func main() {
	b, err := os.ReadFile("/etc/hostname")
	if err != nil {
		log.Fatal(err)
	}
	_ = b
}
EOF

# The fix must hold: staticcheck is clean.
if staticcheck ./... 2>&1 | grep -q "SA1019"; then
	echo "FAIL: SA1019 still present after the rewrite"
	exit 1
fi

echo "OK: fix verified (staticcheck clean after os.ReadFile rewrite)"
exit 0
