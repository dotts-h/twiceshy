// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

// The rollback runbook (#0051, GO_LIVE_HARDENING_PLAN §C5) must stay anchored to
// the real recovery levers. This guard fails if the runbook is missing or omits
// a lever, and — for the commands it cites — asserts they are still live
// subcommands, so a rename can't silently rot the runbook into wrong commands.

const rollbackRunbook = "../../docs/ROLLBACK_RUNBOOK.md"

func TestRollbackRunbookDocumentsTheRealLevers(t *testing.T) {
	b, err := os.ReadFile(rollbackRunbook)
	if err != nil {
		t.Fatalf("rollback runbook must exist at %s: %v", rollbackRunbook, err)
	}
	doc := string(b)

	for _, lever := range []string{
		"TWICESHY_PAUSE", // emergency stop
		"repromote",      // single-record restore (#0048)
		"git revert",     // batch rollback
		"-effect",        // dry-run preview before acting (#0049)
		"ADR-0013",       // decision cross-link
		"SCHEMA",         // lifecycle cross-link
	} {
		if !strings.Contains(doc, lever) {
			t.Errorf("rollback runbook omits the %q lever", lever)
		}
	}
}

// The restore command the runbook tells an operator to run must still dispatch —
// not be an unknown subcommand.
func TestRollbackRunbookRestoreCommandIsLive(t *testing.T) {
	err := run(context.Background(), []string{"repromote"}, &bytes.Buffer{}, noEnv)
	if err == nil {
		t.Fatal("repromote with no args should error (missing -id)")
	}
	if strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("runbook cites `repromote` but it is not a live subcommand: %v", err)
	}
}
