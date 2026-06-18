// SPDX-License-Identifier: AGPL-3.0-only

// Package doctor holds twiceshy's store-hygiene jobs (ADR-0001 §7): background
// checks that keep the corpus honest. Doctors are **delta-only** — they operate
// per record and **report/propose** changes; they never rewrite the store and
// never silently mutate a record (git/PR is the trust boundary, ADR-0008).
//
// Per ADR-0010 only the framework + D2 (staleness) are built now; D1/D3/D4/D5
// are deferred until their substrate (runnable repros, usage counters, a larger
// corpus) exists.
package doctor

import (
	"context"

	"github.com/dotts-h/twiceshy/internal/record"
)

// Finding is one proposed delta a doctor surfaces for human review.
type Finding struct {
	RecordID string
	Path     string
	Issue    string // what is wrong
	Proposal string // the suggested change (e.g. "mark stale; set valid.until")
}

// Report is a doctor's output for one run.
type Report struct {
	Doctor   string
	Findings []Finding
}

// Doctor is a delta-only, report-only store-hygiene job.
type Doctor interface {
	Name() string
	// Run inspects the records and returns proposed deltas. It MUST NOT mutate
	// the records or the corpus.
	Run(ctx context.Context, recs []*record.Record) (Report, error)
}

// Cycle is one release cycle of an endoflife.date product.
type Cycle struct {
	Cycle string // e.g. "1.21"
	EOL   string // "YYYY-MM-DD" if known; "" when unknown / not-yet-EOL
}

// EOLSource yields the release cycles (with end-of-life dates) for a product.
// It is a seam so tests can stub it — the real impl hits endoflife.date, which
// CI must never call.
type EOLSource interface {
	Cycles(ctx context.Context, product string) ([]Cycle, error)
}
