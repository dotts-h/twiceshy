// SPDX-License-Identifier: AGPL-3.0-only

package main

// Test-only aliases for the corpus drivers that moved to internal/run (ADR-0023).
// A few cmd-level suites still exercise the drivers through the command boundary
// (failsafe wiring, dispute intake), so they reach the moved functions by these
// aliases rather than importing the package under their own name. The drivers'
// own unit tests live with the code in internal/run.
import runpkg "github.com/dotts-h/twiceshy/internal/run"

var (
	promoteCorpus  = runpkg.PromoteCorpus
	adaptCorpus    = runpkg.AdaptCorpus
	reportDisputes = runpkg.ReportDisputes
)
