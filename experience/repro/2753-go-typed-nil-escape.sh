#!/usr/bin/env sh
# F2P repro for exp-2753 (positive): a nil pointer returned as a Go `error` is a
# NON-nil interface — the "typed nil" trap — whereas a literal nil error is truly
# nil. Demonstrates both the trap and the escape in one run.
#
# Fail-to-pass discipline (docs/SCHEMA.md, guard.repros kind: positive):
#   exit 0  = trap reproduced (typed-nil error != nil) AND escape works (literal-nil error == nil)
#   exit 1  = the world changed (Go altered interface-nil semantics) -> stale
#   exit 75 = environment cannot run the repro (no go toolchain) -> skip
#
# Self-contained and OFFLINE: stdlib-only (errors, fmt), no network, no deps.
# Designed for the twiceshy sandbox broker, whose /work volume is writable +
# exec-able and where TMPDIR points at /work (the toolchain compiles+execs from
# TMPDIR — see exp-0017).
set -u

command -v go >/dev/null 2>&1 || { echo "SKIP: go toolchain not available"; exit 75; }

export GOTOOLCHAIN=local GOFLAGS=-mod=mod
export GOCACHE=/work/.gocache GOPATH=/work/.gopath
mod=/work/typednil-escape
mkdir -p "$mod" || { echo "SKIP: cannot create work module dir"; exit 75; }
cd "$mod" || exit 75

cat > go.mod <<'EOM'
module typednil

go 1.25
EOM

cat > main.go <<'EOM'
package main

import (
	"errors"
	"fmt"
	"os"
)

// ValidationError is a custom error type; *ValidationError implements error.
type ValidationError struct{ Field string }

func (e *ValidationError) Error() string { return "invalid field: " + e.Field }

// validateTrap reproduces the trap: it declares a typed nil *ValidationError and
// returns it as an error. The returned interface is (type=*ValidationError,
// value=nil) — which is NOT nil, because an interface value is nil only when both
// its type and its value are nil (Go spec, "Comparison operators").
func validateTrap(ok bool) error {
	var e *ValidationError // a nil pointer of concrete type *ValidationError
	if !ok {
		e = &ValidationError{Field: "name"}
	}
	return e // BUG: when ok==true and e==nil, the returned error is still != nil
}

// validateEscape is the fix: return an untyped nil literally in the no-error
// path, so a "no error" result is a genuine nil interface.
func validateEscape(ok bool) error {
	if !ok {
		return &ValidationError{Field: "name"}
	}
	return nil
}

func main() {
	// Trap: a "no error" result that nonetheless compares != nil.
	if err := validateTrap(true); err == nil {
		fmt.Println("NOT REPRODUCED: a typed-nil *ValidationError returned as error compared == nil")
		os.Exit(1)
	}
	// Escape: a real nil error.
	if err := validateEscape(true); err != nil {
		fmt.Printf("ESCAPE BROKEN: a literal-nil error compared != nil (%v)\n", err)
		os.Exit(1)
	}
	// The same trap reached via errors.Is: errors.Is(x, nil) is just x == nil.
	if errors.Is(validateTrap(true), nil) {
		fmt.Println("NOT REPRODUCED: errors.Is(typed-nil, nil) returned true")
		os.Exit(1)
	}
	fmt.Println("OK: typed-nil error != nil (trap); literal-nil error == nil (escape)")
}
EOM

if go run . ; then
	exit 0
fi
echo "REPRO FAILED: the typed-nil trap/escape assertions did not hold"
exit 1
