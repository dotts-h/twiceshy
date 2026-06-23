//go:build livecorpus

// SPDX-License-Identifier: AGPL-3.0-only

package doctor_test

import (
	"context"
	"os"
	"testing"

	"github.com/dotts-h/twiceshy/internal/doctor"
	"github.com/dotts-h/twiceshy/internal/record"
)

func TestStaleness_RealCorpusNotFalseFlagged(t *testing.T) {
	if _, err := os.Stat("../../experience"); err != nil {
		t.Skip("live corpus not present at ../.. (decoupled to twiceshy-corpus, ADR-0021)")
	}
	recs, err := record.LoadCorpus("../..")
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	// A populated EOL source for every mapped product — the committed corpus's
	// VALIDATED records must not trip signal 2 (EOL'd runtime Fixed-cycle) nor
	// signal 1 (past valid.until). Quarantined imports (incl. EOL-runtime
	// advisories) are exempt: the doctor evaluates only validated records.
	eol := stubEOL{
		"go":     {{Cycle: "1.16", EOL: "2022-08-01"}, {Cycle: "1.20", EOL: "2024-02-01"}},
		"python": {{Cycle: "3.8", EOL: "2024-10-01"}},
		"nodejs": {{Cycle: "16", EOL: "2023-09-01"}},
	}
	rep, _ := doctor.NewStaleness(eol, fixedNow).Run(context.Background(), recs)
	if len(rep.Findings) != 0 {
		t.Fatalf("committed corpus false-flagged as stale: %v", rep.Findings)
	}
}
