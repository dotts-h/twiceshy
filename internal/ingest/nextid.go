// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"context"
	"fmt"

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
	}
	return record.FormatID(n), nil
}
