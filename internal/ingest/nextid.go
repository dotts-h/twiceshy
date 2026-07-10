// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// NextID allocates the next exp-NNNN id for a new record, robust against a live
// index that has drifted behind the committed corpus. It returns one past the
// maximum of (a) the index's highest id and (b) the highest id in the
// source-of-truth corpus tree at corpusRoot — so a stale index can never hand
// back an id that already exists on disk (#0059). Pass corpusRoot == "" to use
// the index alone (e.g. an in-memory test index with no disk backing).
//
// This closes the stale-index hazard only; it does NOT make allocation atomic,
// so two concurrent callers can still observe the same max (TECH_DEBT M3). The
// write path is propose-only — a colliding draft is caught in PR review — so
// that residual race is acceptable until an unattended write path exists.
func NextID(ctx context.Context, ix *index.Index, corpusRoot string) (string, error) {
	return NextIDWithBase(ctx, ix, corpusRoot, "")
}

// NextIDWithBase is NextID plus a merge-safe base-ref high-water mark and
// external floors. It returns one past the maximum id visible in the index,
// the local corpus tree, baseRef (when non-empty), and is strictly above
// every floor (intended floor source is the open-PR scan, #0121).
// So a branch with stale local files or parallel drafts cannot allocate an id
// already present elsewhere.
func NextIDWithBase(ctx context.Context, ix *index.Index, corpusRoot, baseRef string, floors ...int) (string, error) {
	idxNext, err := ix.NextID(ctx)
	if err != nil {
		return "", err
	}
	n, ok := record.Num(idxNext)
	if !ok {
		return "", fmt.Errorf("index returned malformed next id %q", idxNext)
	}
	if corpusRoot != "" {
		diskMax, err := record.MaxID(corpusRoot)
		if err != nil {
			return "", fmt.Errorf("scanning corpus for max id: %w", err)
		}
		if diskMax+1 > n {
			n = diskMax + 1
		}
		if baseRef != "" {
			baseMax, err := maxIDAtRef(ctx, corpusRoot, baseRef)
			if err != nil {
				return "", fmt.Errorf("scanning base ref for max id: %w", err)
			}
			if baseMax+1 > n {
				n = baseMax + 1
			}
		}
	}
	for _, floor := range floors {
		if floor+1 > n {
			n = floor + 1
		}
	}
	return record.FormatID(n), nil
}

func maxIDAtRef(ctx context.Context, repo, ref string) (int, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-tree", "-r", "--name-only", ref, "--", "experience")
	cmd.Dir = repo
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return 0, fmt.Errorf("%v: %s", err, msg)
		}
		return 0, err
	}
	max := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		path := filepath.ToSlash(strings.TrimSpace(line))
		if path == "" {
			continue
		}
		id := filepath.Base(path)
		dash := strings.IndexByte(id, '-')
		if dash < 0 {
			continue
		}
		n, ok := record.Num("exp-" + id[:dash])
		if ok && n > max {
			max = n
		}
	}
	return max, nil
}
