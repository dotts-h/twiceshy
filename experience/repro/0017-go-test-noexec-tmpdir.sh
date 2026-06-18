#!/usr/bin/env sh
# F2P repro for exp-0017: `go test` fails with "permission denied" when TMPDIR is
# on a noexec filesystem, because the toolchain compiles the test binary into
# TMPDIR and then execs it.
#
# Fail-to-pass discipline (docs/SCHEMA.md, guard.repro):
#   exit 0  = trap reproduced (noexec TMPDIR fails) AND escape works (exec TMPDIR passes)
#   exit 1  = the world changed (trap gone, or the fix broke)
#   exit 75 = environment cannot run the repro (no go / /tmp is not noexec) — skip
#
# This is self-contained and runs OFFLINE: a stdlib-only module, no network. It
# is designed to run inside the twiceshy sandbox broker, whose /tmp is mounted
# noexec and whose /work volume is writable + exec-able.
set -u

command -v go >/dev/null 2>&1 || { echo "SKIP: go toolchain not available"; exit 75; }

export GOTOOLCHAIN=local GOFLAGS=-mod=mod
export GOCACHE=/work/.gocache GOPATH=/work/.gopath
mod=/work/m
mkdir -p "$mod" || { echo "SKIP: cannot create work module dir"; exit 75; }

cat > "$mod/go.mod" <<'EOM'
module reprotest

go 1.25
EOM
cat > "$mod/x_test.go" <<'EOM'
package reprotest

import "testing"

// A trivial stdlib-only test: building+running it is enough to exercise the
// toolchain's compile-to-TMPDIR-then-exec step.
func TestTrivial(t *testing.T) {
	if 1+1 != 2 {
		t.Fatal("arithmetic broke")
	}
}
EOM
cd "$mod" || exit 75

# Confirm /tmp is really noexec here; otherwise the trap can't be demonstrated.
probe=/tmp/twiceshy-noexec-probe
printf '#!/bin/sh\nexit 0\n' > "$probe" 2>/dev/null && chmod +x "$probe" 2>/dev/null
if "$probe" >/dev/null 2>&1; then
	echo "SKIP: /tmp is exec-able here; cannot demonstrate the noexec trap"
	exit 75
fi

# Trap: with TMPDIR on the noexec /tmp, the test binary can't be exec'd.
if TMPDIR=/tmp go test ./... >trap.out 2>&1; then
	echo "NOT REPRODUCED: go test passed with a noexec TMPDIR"
	cat trap.out
	exit 1
fi
if ! grep -qi "permission denied" trap.out; then
	echo "NOT REPRODUCED: go test failed for a different reason:"
	cat trap.out
	exit 1
fi

# Escape: point TMPDIR at the exec-able /work volume.
exectmp=/work/.tmp
mkdir -p "$exectmp" || { echo "SKIP: cannot create exec tmpdir"; exit 75; }
if TMPDIR="$exectmp" go test ./... >fix.out 2>&1; then
	echo "OK: noexec TMPDIR -> permission denied; exec TMPDIR -> pass"
	exit 0
fi
echo "FIX BROKEN: go test failed even with an exec-able TMPDIR:"
cat fix.out
exit 1
