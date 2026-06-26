// SPDX-License-Identifier: AGPL-3.0-only

package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotts-h/twiceshy/internal/drafter"
	"github.com/dotts-h/twiceshy/internal/record"
)

// pipelineRunner is the seam the draft command drives: drafter.Pipeline.Run
// satisfies it. Abstracting it lets the corpus walk + selection + persistence be
// unit-tested without Docker/runsc (the broker is the part that needs them).
type pipelineRunner interface {
	Run(ctx context.Context, rec *record.Record) (drafter.Outcome, error)
}

// DraftStats summarizes a draft run.
type DraftStats struct {
	Attached    int // a drafted repro held under the gate and was attached
	Rejected    int // a drafted repro did not hold (auto-rejected, files removed)
	Unsupported int // no template covered the record (left for the model drafter)
	Skipped     int // a quarantined record already carried a positive proof (idempotent re-run)
}

// IsCandidate reports whether the drafter should attempt rec: a quarantined
// record that does not already carry a positive (fail-to-pass) proof. The same
// predicate drives the -dry-run listing and the real walk, so the preview can
// never diverge from what the gate actually touches. A record with only a
// negative (dead-end) repro is still a candidate — it lacks a positive proof.
func IsCandidate(rec *record.Record) bool {
	return rec.Status == "quarantined" && !record.HasPositiveRepro(rec)
}

// DraftCorpus is the testable core of `twiceshy draft`: it walks the candidate
// records, runs each through the pipeline, and persists the record whose drafted
// repro held (the pipeline already wrote/removed the repro files and mutated the
// guard in place). run and persist are injected so the walk is exercised without
// a sandbox. A gate error aborts; records attached before it stay written (each
// is an independently-valid proven repro, and a re-run resumes — already-proven
// records are skipped).
func DraftCorpus(ctx context.Context, corpus string, recs []*record.Record, run pipelineRunner, persist func(string, *record.Record) error, out io.Writer) (DraftStats, error) {
	var st DraftStats
	for _, rec := range recs {
		if !IsCandidate(rec) {
			if rec.Status == "quarantined" {
				st.Skipped++ // already carries a positive proof — re-running attaches nothing new
			}
			continue
		}
		outcome, err := run.Run(ctx, rec)
		if err != nil {
			return st, fmt.Errorf("draft %s: %w", rec.ID, err)
		}
		if !outcome.Drafted {
			st.Unsupported++
			continue
		}
		if !outcome.Attached {
			st.Rejected++
			_, _ = fmt.Fprintf(out, "  rejected %s (%s)\n", rec.ID, outcome.Reason)
			continue
		}
		if err := persist(corpus, rec); err != nil {
			// The drafter wrote the repro dir and the gate proved it, but the record
			// that references it never landed — remove the now-orphan repro so a
			// failed persist leaves no dangling files in the corpus.
			removeRepro(corpus, outcome.ReproPath)
			return st, fmt.Errorf("persist %s: %w", rec.ID, err)
		}
		st.Attached++
		_, _ = fmt.Fprintf(out, "  attached %s -> %s\n", rec.ID, outcome.ReproPath)
	}
	return st, nil
}

// removeRepro best-effort deletes a drafted repro directory under the corpus,
// used to roll back a proven-but-unpersisted draft.
func removeRepro(corpus, reproPath string) {
	if reproPath == "" {
		return
	}
	if dst, err := safeJoin(corpus, reproPath); err == nil {
		_ = os.RemoveAll(dst)
	}
}

func safeJoin(base, rel string) (string, error) {
	clean := filepath.FromSlash(rel)
	dst := filepath.Join(base, clean)
	rp, err := filepath.Rel(filepath.Clean(base), dst)
	if filepath.IsAbs(clean) || err != nil || rp == ".." || strings.HasPrefix(rp, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing path that escapes the output root: %q", rel)
	}
	return dst, nil
}
