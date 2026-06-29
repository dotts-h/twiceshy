// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/retro"
	"github.com/dotts-h/twiceshy/internal/spool"
	"github.com/dotts-h/twiceshy/internal/telemetry"
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
	base := fs.String("base", "", "base git ref for merge-safe id allocation")
	telemetryLog := fs.String("telemetry-log", getenv("TWICESHY_TELEMETRY_LOG"), "decision log for served-vs-used helpfulness join (empty = disabled)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *queue == "" {
		return errors.New("retro-intake requires -queue <dir> (the directory serve enqueues transcripts into)")
	}

	// Build the analyzer first: a misconfigured endpoint should fail fast, before
	// the (slower) index build.
	cfg, err := modelConfigFromEnv(getenv, *model, *maxTraps)
	if err != nil {
		return err
	}
	analyzer, err := retro.NewModelAnalyzer(cfg)
	if err != nil {
		return err
	}

	ix, _, err := buildIndex(ctx, c, false)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	var join *helpfulJoin
	if *telemetryLog != "" {
		usageJudge, err := retro.NewModelUsageJudge(cfg)
		if err != nil {
			return err
		}
		// Mirror the serve's salt resolution EXACTLY (telemetrySalt): explicit salt,
		// else the bearer token. The serve writes the log salted this way; if the drain
		// used a different salt the session hashes diverge and the join matches nothing
		// (#0098). Requires TWICESHY_TOKEN in the drain's env (scheduled-retro.sh sources
		// the brain secret store) — the same token the serve runs with.
		salt := telemetrySalt(getenv("TWICESHY_TELEMETRY_SALT"), getenv("TWICESHY_TOKEN"))
		logPath := *telemetryLog
		join = &helpfulJoin{
			judge: usageJudge,
			rec:   ix,
			servedFor: func(sid string) (map[string]bool, error) {
				return telemetry.ServedInSession(logPath, telemetry.Hash([]byte(salt), sid))
			},
		}
	}

	return drainRetro(ctx, analyzer, ix, c.repo, c.corpus, *queue, retroOpts{
		limit:  *limit,
		dryRun: *dryRun,
		now:    time.Now().UTC().Format("2006-01-02"),
		base:   *base,
	}, join, out)
}

// modelConfigFromEnv resolves the off-pool endpoint and model from the environment
// (reusing the judge shim by default) or the -analyzer-model flag.
func modelConfigFromEnv(getenv func(string) string, model string, maxTraps int) (retro.ModelConfig, error) {
	url := getenv("TWICESHY_RETRO_URL")
	if url == "" {
		url = getenv("TWICESHY_JUDGE_URL")
	}
	if url == "" {
		return retro.ModelConfig{}, errors.New("retro-intake requires TWICESHY_RETRO_URL (or TWICESHY_JUDGE_URL): the off-pool analyzer endpoint")
	}
	if model == "" {
		model = getenv("TWICESHY_RETRO_MODEL")
	}
	if model == "" {
		model = getenv("TWICESHY_JUDGE_MODEL")
	}
	if model == "" {
		return retro.ModelConfig{}, errors.New("retro-intake requires -analyzer-model (or TWICESHY_RETRO_MODEL / TWICESHY_JUDGE_MODEL)")
	}
	return retro.ModelConfig{Endpoint: url, Model: model, MaxTraps: maxTraps}, nil
}

// helpfulJoin bundles the served-vs-used attribution seam (#0069). nil = disabled.
type helpfulJoin struct {
	judge     retro.UsageJudge
	rec       retro.ConfirmHelpfuler
	servedFor func(sessionID string) (map[string]bool, error)
}

// retroOpts bounds one drain.
type retroOpts struct {
	limit  int
	dryRun bool
	now    string // YYYY-MM-DD stamped on created records
	base   string // optional base ref for merge-safe id allocation
}

const analyzeAttempts = 3

// drainRetro is the testable core: analyze each spooled transcript and materialize
// its candidates as quarantined drafts. The Analyzer is injected so tests drive it
// with a StubAnalyzer (no network). Fail-safe: an analyzer error leaves the
// transcript queued and stops (a scheduled run retries); a transcript is dequeued
// only after it is successfully analyzed.
func drainRetro(ctx context.Context, analyzer retro.Analyzer, ix *index.Index, repo, corpus, queue string, opts retroOpts, join *helpfulJoin, out io.Writer) error {
	if _, err := record.LoadCorpus(corpus); err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}
	files, err := spool.List(queue)
	if err != nil {
		return fmt.Errorf("listing retro queue: %w", err)
	}

	id, err := nextIDForCorpus(ctx, corpus, opts.base)
	if err != nil {
		return fmt.Errorf("allocating next id: %w", err)
	}
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

		var candidates []retro.Candidate
		var analyzeErr error
		for attempt := 0; attempt < analyzeAttempts; attempt++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			candidates, analyzeErr = analyzer.Analyze(ctx, tr.Transcript)
			if analyzeErr == nil {
				break
			}
			if !errors.Is(analyzeErr, retro.ErrUnprocessable) {
				return fmt.Errorf("retro-intake: analyze %s: %w", base, analyzeErr)
			}
		}
		if analyzeErr != nil {
			dead := filepath.Join(queue, "dead")
			if mkErr := os.MkdirAll(dead, 0o755); mkErr == nil {
				_ = os.Rename(f, filepath.Join(dead, base))
			}
			_, _ = fmt.Fprintf(out, "  skip %s: unprocessable after %d attempts (%v)\n", base, analyzeAttempts, analyzeErr)
			skipped++
			continue
		}

		author := tr.Author
		if strings.TrimSpace(author) == "" {
			author = "retro-capture"
		}

		for _, cand := range candidates {
			draft := candidateDraft(cand)
			key := batchKey(draft)
			if seen[key] {
				dup++
				continue
			}
			meta := ingest.Meta{ID: id, Author: author, Now: opts.now, IncludeQuarantined: true}
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
			id = bumpID(id)
		}

		if join != nil && tr.SessionID != "" && !opts.dryRun {
			served, err := join.servedFor(tr.SessionID)
			if err != nil {
				slog.Warn("retro helpfulness join: served lookup failed", "session", tr.SessionID, "file", base, "error", err)
			} else {
				verdicts, err := join.judge.JudgeUsage(ctx, tr.Transcript)
				if err != nil {
					slog.Warn("retro helpfulness join: usage judge failed", "session", tr.SessionID, "file", base, "error", err)
				} else {
					n, err := retro.RecordHelpfulnessAttributed(ctx, join.rec, verdicts, served)
					if err != nil {
						slog.Warn("retro helpfulness join: confirm failed", "session", tr.SessionID, "file", base, "confirmed", n, "error", err)
					} else {
						_, _ = fmt.Fprintf(out, "  confirmed %d helpful (from %s)\n", n, base)
					}
				}
			}
		}

		// Dequeue the transcript only after it is FULLY processed (dry-run dequeues
		// nothing). Dequeuing only when complete is what makes a re-run safe: a
		// signature-less draft (a convention, or a trap with no error_signatures)
		// never fingerprints, so ingest.Prepare returns it as Similar — not Known — on
		// a re-analysis; a half-processed transcript left queued would therefore have
		// its candidates re-written as duplicate drafts.
		if !opts.dryRun {
			_ = spool.Remove(f)
		}
		// Soft record limit, enforced at the transcript boundary so no transcript is
		// ever left half-processed: stop before the NEXT transcript once enough has
		// been written this run (may overshoot by the current transcript's candidates).
		if opts.limit > 0 && created >= opts.limit {
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
