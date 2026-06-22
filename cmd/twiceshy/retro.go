// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/retro"
	"github.com/dotts-h/twiceshy/internal/spool"
)

// runRetroIntake drains the session-retro queue (#0065, ADR-0018): for each
// spooled transcript it runs the off-pool analyzer, then feeds every extracted
// candidate through the same quarantine ladder the importer uses (ingest.Prepare →
// writeRecord). Nothing is born validated — the analyzer drafts, never promotes.
// The expensive analysis lives here, off the request path; the /retro endpoint
// only screens and spools.
func runRetroIntake(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("retro-intake", flag.ContinueOnError)
	c := addCommonFlags(fs)
	queue := fs.String("queue", "", "retro queue directory written by `serve -retro-queue` (required)")
	model := fs.String("analyzer-model", "", "off-pool analyzer model id (default: TWICESHY_RETRO_MODEL, else TWICESHY_JUDGE_MODEL)")
	limit := fs.Int("limit", 0, "max new records to write this run (0 = unlimited); bounds a scheduled drain")
	maxTraps := fs.Int("max-traps", 0, "max candidates accepted per transcript (0 = default)")
	dryRun := fs.Bool("dry-run", false, "analyze and report, but write nothing and dequeue nothing")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *queue == "" {
		return errors.New("retro-intake requires -queue <dir> (the directory serve enqueues transcripts into)")
	}

	// Build the analyzer first: a misconfigured endpoint should fail fast, before
	// the (slower) index build.
	analyzer, err := analyzerFromEnv(getenv, *model, *maxTraps)
	if err != nil {
		return err
	}

	ix, _, err := buildIndex(ctx, c, false)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	return drainRetro(ctx, analyzer, ix, c.repo, c.corpus, *queue, retroOpts{
		limit:  *limit,
		dryRun: *dryRun,
		now:    time.Now().UTC().Format("2006-01-02"),
	}, out)
}

// analyzerFromEnv builds the off-pool analyzer edge. The endpoint and model come
// from the environment (reusing the judge shim by default) or the -analyzer-model
// flag; the analyzer is a drafter (quarantined output), so — unlike the judge — no
// model family is forbidden.
func analyzerFromEnv(getenv func(string) string, model string, maxTraps int) (retro.Analyzer, error) {
	url := getenv("TWICESHY_RETRO_URL")
	if url == "" {
		url = getenv("TWICESHY_JUDGE_URL")
	}
	if url == "" {
		return nil, errors.New("retro-intake requires TWICESHY_RETRO_URL (or TWICESHY_JUDGE_URL): the off-pool analyzer endpoint")
	}
	if model == "" {
		model = getenv("TWICESHY_RETRO_MODEL")
	}
	if model == "" {
		model = getenv("TWICESHY_JUDGE_MODEL")
	}
	if model == "" {
		return nil, errors.New("retro-intake requires -analyzer-model (or TWICESHY_RETRO_MODEL / TWICESHY_JUDGE_MODEL)")
	}
	return retro.NewModelAnalyzer(retro.ModelConfig{Endpoint: url, Model: model, MaxTraps: maxTraps})
}

// retroOpts bounds one drain.
type retroOpts struct {
	limit  int
	dryRun bool
	now    string // YYYY-MM-DD stamped on created records
}

// drainRetro is the testable core: analyze each spooled transcript and materialize
// its candidates as quarantined drafts. The Analyzer is injected so tests drive it
// with a StubAnalyzer (no network). Fail-safe: an analyzer error leaves the
// transcript queued and stops (a scheduled run retries); a transcript is dequeued
// only after it is successfully analyzed.
func drainRetro(ctx context.Context, analyzer retro.Analyzer, ix *index.Index, repo, corpus, queue string, opts retroOpts, out io.Writer) error {
	recs, err := record.LoadCorpus(corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	files, err := spool.List(queue)
	if err != nil {
		return fmt.Errorf("listing retro queue: %w", err)
	}

	next := maxRecordNum(recs)
	seen := map[string]bool{} // within-run dedup, keyed like the importer's batch
	created, dup, skipped := 0, 0, 0
	for _, f := range files {
		base := filepath.Base(f)
		tr, err := spool.ReadTranscript(f)
		if err != nil || strings.TrimSpace(tr.Transcript) == "" {
			// Unreadable or bodyless (e.g. a misrouted report queue) — skip and
			// LEAVE queued; we never delete an entry we did not process.
			_, _ = fmt.Fprintf(out, "  skip %s: unreadable or empty transcript\n", base)
			skipped++
			continue
		}

		candidates, err := analyzer.Analyze(ctx, tr.Transcript)
		if err != nil {
			// Transient (the off-pool endpoint is down): leave this and the rest
			// queued and stop so a scheduled run alerts and retries — never drop.
			return fmt.Errorf("retro-intake: analyze %s: %w", base, err)
		}

		author := tr.Author
		if strings.TrimSpace(author) == "" {
			author = "retro-capture"
		}

		limitHit := false
		for _, cand := range candidates {
			draft := candidateDraft(cand)
			key := batchKey(draft)
			if seen[key] {
				dup++
				continue
			}
			meta := ingest.Meta{ID: fmt.Sprintf("exp-%04d", next+1), Author: author, Now: opts.now, IncludeQuarantined: true}
			if tr.SessionID != "" {
				s := tr.SessionID
				meta.Session = &s
			}
			outcome, err := ingest.Prepare(ctx, ix, repo, draft, meta)
			if err != nil {
				_, _ = fmt.Fprintf(out, "  skip candidate %q: %v\n", cand.Title, err)
				continue
			}
			if outcome.Record == nil { // Known — already in the corpus
				dup++
				continue
			}
			seen[key] = true
			rec := outcome.Record
			if opts.dryRun {
				_, _ = fmt.Fprintf(out, "  would create %s %s (from %s)\n", rec.ID, rec.Path, base)
			} else {
				if err := writeRecord(corpus, rec); err != nil {
					return fmt.Errorf("writing draft from %s: %w", base, err)
				}
				_, _ = fmt.Fprintf(out, "  created %s %s (from %s)\n", rec.ID, rec.Path, base)
			}
			created++
			next++
			if opts.limit > 0 && created >= opts.limit {
				limitHit = true
				break
			}
		}

		// Dequeue only after a successful analysis (dry-run dequeues nothing). On a
		// limit break, leave the entry queued — a re-run dedups what we already wrote.
		if !opts.dryRun && !limitHit {
			_ = spool.Remove(f)
		}
		if limitHit {
			_, _ = fmt.Fprintf(out, "retro-intake: record limit %d reached\n", opts.limit)
			break
		}
	}

	verb := "created"
	if opts.dryRun {
		verb = "would create"
	}
	_, _ = fmt.Fprintf(out, "retro-intake: %s %d draft(s), %d known/duplicate, %d skipped\n", verb, created, dup, skipped)
	return nil
}

// candidateDraft maps an extracted candidate onto an ingest.Draft, exactly as
// record_experience builds one from RecordArgs (internal/server/record.go) — so
// retro-extracted traps and agent-submitted ones travel the identical ladder.
func candidateDraft(c retro.Candidate) ingest.Draft {
	d := ingest.Draft{Kind: c.Kind, Title: c.Title, Body: c.Body}
	if c.Summary != "" || len(c.ErrorSignatures) > 0 {
		d.Symptom = &record.Symptom{Summary: c.Summary, ErrorSignatures: c.ErrorSignatures}
	}
	if c.Ecosystem != "" || c.Package != "" {
		d.AppliesTo = []record.AppliesTo{{Ecosystem: c.Ecosystem, Package: c.Package}}
	}
	if c.RootCause != "" || c.Fix != "" {
		d.Resolution = &record.Resolution{RootCause: c.RootCause, Fix: c.Fix}
	}
	return d
}
