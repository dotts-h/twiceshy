// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// ImportStats summarizes a batch import run.
type ImportStats struct {
	Created int
	Skipped int
	Flagged int
	Invalid int // drafts skipped because they failed schema validation (#0134)
}

// ImportBatch deduplicates drafts against the corpus and within the batch, then
// persists new quarantined records via persist. Printing goes to out so the CLI
// edge can keep byte-identical ingest output.
func ImportBatch(ctx context.Context, ix *index.Index, repo, corpus, sourceName string, drafts []Draft, startID string, author, now string, dryRun bool, limit int, persist func(string, *record.Record) error, out io.Writer) (ImportStats, error) {
	var st ImportStats
	id := startID
	seen := map[string]bool{} // within-batch dedup, keyed by the primary signal
	for _, d := range drafts {
		key := BatchKey(d)
		if seen[key] {
			st.Skipped++
			continue
		}
		outcome, err := Prepare(ctx, ix, repo, d,
			Meta{ID: id, Author: author, Now: now, IncludeQuarantined: true})
		if err != nil {
			// A malformed single entry (bad title, failed schema check) is a
			// per-entry data defect: skip it, count it, and log it so the drop is
			// never silent — but do not let it abort the whole feed. Infra errors
			// (index failure, context cancellation) are not ErrInvalidDraft and
			// still abort loudly (#0134/#0142; no silent partials, #0119).
			if errors.Is(err, ErrInvalidDraft) {
				st.Invalid++
				_, _ = fmt.Fprintf(out, "  skipped %q: invalid draft (%v)\n", d.Title, err)
				continue
			}
			return st, fmt.Errorf("ingest %q: %w", d.Title, err)
		}
		if outcome.Record == nil { // Known — already in the corpus
			st.Skipped++
			continue
		}
		seen[key] = true
		rec := outcome.Record
		flag := ""
		if len(rec.Provenance.SecurityFlags) > 0 {
			st.Flagged++
			flag = fmt.Sprintf("  [FLAGGED: %s]", strings.Join(rec.Provenance.SecurityFlags, ", "))
		}
		if dryRun {
			_, _ = fmt.Fprintf(out, "  would create %s %s%s\n", rec.ID, rec.Path, flag)
		} else {
			if err := persist(corpus, rec); err != nil {
				return st, err
			}
			_, _ = fmt.Fprintf(out, "  created %s %s%s\n", rec.ID, rec.Path, flag)
		}
		st.Created++
		id = BumpID(id)
		if limit > 0 && st.Created >= limit {
			break // bound a scheduled import so it grows the corpus gradually (0022)
		}
	}

	verb := "created"
	if dryRun {
		verb = "would create"
	}
	_, _ = fmt.Fprintf(out, "ingest %s: %s %d records, skipped %d (known), flagged %d (quarantined+documented), invalid %d (skipped)\n",
		sourceName, verb, st.Created, st.Skipped, st.Flagged, st.Invalid)
	return st, nil
}

// BatchKey is a draft's primary dedup signal for within-batch deduplication:
// its first error signature, else its title. Shared by every caller that
// dedups a batch of drafts before writing (the importer, retro-intake,
// intake-reports) so the dedup signal can never drift between them.
func BatchKey(d Draft) string {
	if d.Symptom != nil {
		for _, sig := range d.Symptom.ErrorSignatures {
			if s := strings.TrimSpace(sig); s != "" {
				return s
			}
		}
	}
	return d.Title
}

// BumpID returns the next sequential exp-NNNN id. The index is not rebuilt
// mid-batch, so ids are advanced locally as records are created.
func BumpID(id string) string {
	n, _ := record.Num(id)
	return record.FormatID(n + 1)
}
