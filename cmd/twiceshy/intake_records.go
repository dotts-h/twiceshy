// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/spool"
)

// runIntakeRecords drains the record spool (ADR-0030 phase 2, #0139): each spooled
// record draft is prepared against the live corpus and, if not a known duplicate,
// written to experience/ as a quarantined record file. Ids are allocated sequentially
// against the live corpus, so draft files spooled before this drain never collide.
// Malformed spool entries are skipped and removed, and write failures abort to keep
// the draft spooled for retry.
func runIntakeRecords(args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("intake-records", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root (the directory containing experience/)")
	queue := fs.String("queue", "", "record queue directory written by `serve -record-queue` (required)")
	base := fs.String("base", "", "base git ref for merge-safe id allocation")
	openPRs := fs.Bool("open-prs", false, "also allocate ids above records on open corpus PRs (Forgejo API, #0121)")
	repo := fs.String("repo", "", "corpus repository identifier for app-scoped fingerprints")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *queue == "" {
		return errors.New("intake-records requires -queue <dir> (the directory serve enqueues records into)")
	}

	if _, err := record.LoadCorpus(*corpus); err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	files, err := spool.List(*queue)
	if err != nil {
		return fmt.Errorf("listing record queue: %w", err)
	}

	// Build a temporary index to run ingest.Prepare deduplication.
	tmp, err := os.CreateTemp("", "twiceshy-intake-records-*.db")
	if err != nil {
		return err
	}
	dbPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(dbPath) }()

	ix, err := index.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	recs, err := record.LoadCorpus(*corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	if err := ix.Rebuild(context.Background(), recs, *repo); err != nil {
		return fmt.Errorf("building index: %w", err)
	}

	// Idle ticks (empty queue) skip the scan — no network dependency for an
	// id the drain never uses.
	floors, err := openPRFloors(context.Background(), *corpus, *openPRs && len(files) > 0, getenv)
	if err != nil {
		return fmt.Errorf("getting open PR floors: %w", err)
	}
	id, err := ingest.NextIDWithBase(context.Background(), ix, *corpus, *base, floors...)
	if err != nil {
		return fmt.Errorf("allocating next id: %w", err)
	}
	today := time.Now().UTC().Format("2006-01-02")
	intaken, dup, skipped := 0, 0, 0
	for _, f := range files {
		baseName := filepath.Base(f)
		rep, err := spool.ReadRecord(f)
		if err != nil {
			_, _ = fmt.Fprintf(out, "  skip %s: unreadable queue entry (%v)\n", baseName, err)
			_ = spool.Remove(f)
			skipped++
			continue
		}

		// Rebuild ingest.Draft (+Symptom/AppliesTo/Resolution/Guard exactly as record.go builds them from args)
		draft := ingest.Draft{
			Kind:  rep.Kind,
			Title: rep.Title,
			Body:  rep.Body,
		}
		if rep.Summary != "" || len(rep.ErrorSignatures) > 0 {
			draft.Symptom = &record.Symptom{
				Summary:         rep.Summary,
				ErrorSignatures: rep.ErrorSignatures,
			}
		}
		if rep.Ecosystem != "" || rep.Package != "" {
			draft.AppliesTo = []record.AppliesTo{{
				Ecosystem: rep.Ecosystem,
				Package:   rep.Package,
			}}
		}
		if rep.RootCause != "" || rep.Fix != "" {
			draft.Resolution = &record.Resolution{
				RootCause: rep.RootCause,
				Fix:       rep.Fix,
			}
		}
		if rep.GuardingTest != "" {
			gt := rep.GuardingTest
			draft.Guard = &record.Guard{
				GuardingTest: &gt,
			}
		}

		// and ingest.Meta{ID: fresh via ingest.NextID against the live corpus, Author: spooled verbatim (already stamped), Session, Now}
		meta := ingest.Meta{
			ID:     id,
			Author: rep.Author,
			Now:    today,
		}
		if rep.Session != "" {
			s := rep.Session
			meta.Session = &s
		}

		outcome, err := ingest.Prepare(context.Background(), ix, *repo, draft, meta)
		if err != nil {
			_, _ = fmt.Fprintf(out, "  skip %s: invalid record draft (%v)\n", baseName, err)
			_ = spool.Remove(f)
			skipped++
			continue
		}

		if outcome.Novelty == index.NoveltyKnown {
			var dupID string
			if len(outcome.Candidates) > 0 {
				dupID = outcome.Candidates[0].ID
			}
			_, _ = fmt.Fprintf(out, "  skip %s: known duplicate (%s)\n", baseName, dupID)
			_ = spool.Remove(f)
			dup++
			continue
		}

		// write the quarantined record file the way runIngest/runIntakeReports persists records
		if err := writeRecord(*corpus, outcome.Record); err != nil {
			// A write failure is environmental — leave the entry queued for retry.
			return fmt.Errorf("writing quarantined record for %s: %w", outcome.Record.ID, err)
		}

		_ = spool.Remove(f)
		id = ingest.BumpID(id)
		intaken++
		_, _ = fmt.Fprintf(out, "  intake %s -> %s\n", baseName, outcome.Record.ID)
	}
	_, _ = fmt.Fprintf(out, "intake-records: materialized %d record(s) into experience/, %d duplicate, %d skipped\n", intaken, dup, skipped)
	return nil
}
