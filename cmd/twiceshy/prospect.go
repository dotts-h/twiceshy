// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dotts-h/twiceshy/internal/agenteval"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// defaultProspectReportPath is "runs/prospect-<UTC timestamp>.json", a fresh name
// per run so successive prospect runs never clobber each other's report.
func defaultProspectReportPath() string {
	return filepath.Join("runs", "prospect-"+time.Now().UTC().Format("20060102T150405Z")+".json")
}

// runProspect is `twiceshy prospect` (ADR-0029, #0113/#0114): for each eligible
// corpus record it drafts a task, runs it WITHOUT the card, and executably
// verifies the output — a miss re-runs WITH the card to measure whether it
// actually helps. Corpus records are never mutated; this is report-only, plus an
// optional append to a gold file for the #0005 eval (-gold-out). Needs docker +
// the runsc runtime (the verifier's broker) and a model endpoint (the runner and
// drafter), like draft/promote/adapt.
func runProspect(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("prospect", flag.ContinueOnError)
	corpus := fs.String("corpus", "", "corpus root (the directory containing experience/) (required)")
	maxN := fs.Int("max", 10, "max eligible-and-drafted cases to process")
	reportPath := fs.String("report", defaultProspectReportPath(), "path to write the JSON prospect report")
	goldOut := fs.String("gold-out", "", "optional path; when set, merge model-hard cases into this prospect-gold file (#0114)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *corpus == "" {
		return fmt.Errorf("prospect: -corpus is required")
	}

	recs, err := record.LoadCorpus(*corpus)
	if err != nil {
		return fmt.Errorf("loading corpus: %w", err)
	}

	runnerKey := getenv("TWICESHY_AGENTEVAL_KEY")
	if runnerKey == "" {
		runnerKey = getenv("NVIDIA_API_KEY")
	}
	runner, err := agenteval.NewModelRunner(agenteval.RunnerConfig{
		Endpoint: getenv("TWICESHY_AGENTEVAL_URL"),
		Model:    getenv("TWICESHY_AGENTEVAL_MODEL"),
		APIKey:   runnerKey,
	})
	if err != nil {
		return fmt.Errorf("configuring runner: %w", err)
	}

	// The drafter endpoint/model fall back to the runner's when unset — a single
	// off-pool model can serve both roles; TWICESHY_PROSPECT_DRAFTER_* lets an
	// operator point drafting at a different model.
	drafterURL := getenv("TWICESHY_PROSPECT_DRAFTER_URL")
	if drafterURL == "" {
		drafterURL = getenv("TWICESHY_AGENTEVAL_URL")
	}
	drafterModel := getenv("TWICESHY_PROSPECT_DRAFTER_MODEL")
	if drafterModel == "" {
		drafterModel = getenv("TWICESHY_AGENTEVAL_MODEL")
	}
	drafter, err := agenteval.NewModelTaskDrafter(agenteval.DrafterConfig{
		Endpoint: drafterURL,
		Model:    drafterModel,
		APIKey:   runnerKey,
	})
	if err != nil {
		return fmt.Errorf("configuring drafter: %w", err)
	}

	// The verifier's broker runs untrusted repro jobs, so it needs docker + runsc,
	// like draft/promote/adapt's broker (ADR-0013 §A3 preflight: fail fast, not
	// partway through the walk).
	broker := repro.NewBroker(agenteval.VerifierImages())
	if err := broker.Healthy(ctx); err != nil {
		return fmt.Errorf("%w: broker substrate: %v", errPreflight, err)
	}
	verifier := agenteval.NewBrokerVerifier(broker)

	rep, err := agenteval.Prospect(ctx, agenteval.ProspectConfig{
		Records:  recs,
		Runner:   runner,
		Verifier: verifier,
		Drafter:  drafter,
		Max:      *maxN,
	})
	if err != nil {
		return err
	}

	if err := writeProspectReport(*reportPath, rep); err != nil {
		return err
	}
	if *goldOut != "" {
		if err := agenteval.MergeProspectGold(*goldOut, rep.ModelHard); err != nil {
			return fmt.Errorf("merging prospect gold at %s: %w", *goldOut, err)
		}
	}
	printProspectSummary(out, rep, *reportPath)
	return nil
}

// writeProspectReport marshals rep as indented JSON to path, creating any
// missing parent directory (e.g. the default "runs/" under a fresh checkout).
func writeProspectReport(path string, rep agenteval.ProspectReport) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("prospect: creating report dir %s: %w", dir, err)
		}
	}
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("prospect: marshaling report: %w", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("prospect: writing report %s: %w", path, err)
	}
	return nil
}

// onAlsoFailsCount counts the model-hard cases whose ON arm ALSO failed — the
// "card exists but doesn't help" lead (#0114's distinct class) — so the one-screen
// summary makes it visible alongside the plain model-hard count.
func onAlsoFailsCount(rep agenteval.ProspectReport) int {
	n := 0
	for _, c := range rep.ModelHard {
		if !c.OnAvoided {
			n++
		}
	}
	return n
}

// printProspectSummary writes the one-screen human summary: scanned/eligible/
// drafted, skips by reason, and the model-hard / on-also-fails counts.
func printProspectSummary(out io.Writer, rep agenteval.ProspectReport, reportPath string) {
	_, _ = fmt.Fprintf(out, "prospect: scanned %d, eligible %d, drafted %d\n", rep.Scanned, rep.Eligible, rep.Drafted)
	for reason, n := range rep.Skipped {
		_, _ = fmt.Fprintf(out, "  skipped (%s): %d\n", reason, n)
	}
	_, _ = fmt.Fprintf(out, "  off-avoided: %d\n", len(rep.OffAvoided))
	_, _ = fmt.Fprintf(out, "  model-hard: %d (on-also-fails: %d)\n", len(rep.ModelHard), onAlsoFailsCount(rep))
	_, _ = fmt.Fprintf(out, "report written to %s\n", reportPath)
}
