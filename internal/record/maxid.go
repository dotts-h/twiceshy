// SPDX-License-Identifier: AGPL-3.0-only

package record

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
)

// MaxID returns the highest exp-NNNN id present in the on-disk corpus tree under
// root/experience, or 0 if the corpus is empty or absent. Unlike index.NextID —
// a MAX(id)+1 read over the SQLite index, which can lag the filesystem — MaxID
// reads the source of truth (the record files themselves), so id allocation is
// correct even when a live index has drifted behind the committed corpus
// (#0059). It matches the canonical record-path layout only (the same paths
// LoadCorpus walks) and never parses a record body, so a malformed record can
// never break allocation.
func MaxID(root string) (int, error) {
	expDir := filepath.Join(root, "experience")
	max := 0
	err := filepath.WalkDir(expDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// An absent corpus *root* means "no records yet", not a failure. Any
			// other error (incl. a file vanishing mid-walk) propagates, so we never
			// silently under-count and hand back an id that is already taken.
			if errors.Is(walkErr, fs.ErrNotExist) && p == expDir {
				return fs.SkipAll
			}
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		m := reRecordPath.FindStringSubmatch(filepath.ToSlash(rel))
		if m == nil {
			return nil // repro scripts, READMEs, scratch files
		}
		if n, err := strconv.Atoi(m[2]); err == nil && n > max {
			max = n
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return max, nil
}
