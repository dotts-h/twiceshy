#!/usr/bin/env sh
# Dead-end repro for exp-2753 (negative): the tempting-but-wrong fix for the
# typed-nil trap is "just nil-check at the call site" while the callee keeps
# returning a typed nil. It does not work — the caller still takes the error path.
# This proves "don't try Z": fix the RETURN site, not the call site.
#
# Fail-to-pass discipline (docs/SCHEMA.md, guard.repros kind: negative):
#   exit 0  = dead-end still fails as expected (caller-side nil check mis-fires)
#   exit 1  = the dead-end started working (Go changed typed-nil semantics) -> stale
#   exit 75 = environment cannot run the repro (no go toolchain) -> skip
#
# Self-contained and OFFLINE: stdlib-only (fmt), no network, no deps. Built for
# the twiceshy sandbox broker (/work writable+exec-able, TMPDIR=/work).
set -u

command -v go >/dev/null 2>&1 || { echo "SKIP: go toolchain not available"; exit 75; }

export GOTOOLCHAIN=local GOFLAGS=-mod=mod
export GOCACHE=/work/.gocache GOPATH=/work/.gopath
mod=/work/typednil-deadend
mkdir -p "$mod" || { echo "SKIP: cannot create work module dir"; exit 75; }
cd "$mod" || exit 75

cat > go.mod <<'EOM'
module typednildead

go 1.25
EOM

cat > main.go <<'EOM'
package main

import (
	"fmt"
	"os"
)

type ValidationError struct{ Field string }

func (e *ValidationError) Error() string { return "invalid field: " + e.Field }

// callee keeps the bug: it returns a typed-nil *ValidationError on success.
func callee(ok bool) error {
	var e *ValidationError
	if !ok {
		e = &ValidationError{Field: "name"}
	}
	return e
}

func main() {
	// The dead-end: "just nil-check at the call site." Because the callee returns
	// a typed nil on success, the standard `if err != nil` idiom wrongly enters
	// the error branch — the call-site check cannot rescue a typed-nil return.
	tookErrorPath := false
	if err := callee(true); err != nil {
		tookErrorPath = true
		_ = err
	}
	if !tookErrorPath {
		fmt.Println("DEAD-END NO LONGER FAILS: caller-side nil check worked despite a typed-nil callee")
		os.Exit(1)
	}
	fmt.Println("OK: dead-end confirmed — caller-side nil check still mis-fires on a typed-nil callee (fix the callee, not the caller)")
}
EOM

if go run . ; then
	exit 0
fi
echo "REPRO FAILED: the dead-end assertion did not hold"
exit 1
