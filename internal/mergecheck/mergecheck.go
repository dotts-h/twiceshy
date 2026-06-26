// SPDX-License-Identifier: AGPL-3.0-only

// Package mergecheck contains git-backed corpus PR gates. The command edge only
// parses flags; this package owns the merge-time checks.
package mergecheck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dotts-h/twiceshy/internal/record"
)

// MergeParams names the corpus repository and refs to compare.
type MergeParams struct {
	Corpus string
	Base   string
	Head   string
}

var recordPathRE = regexp.MustCompile(`^experience/[0-9]{4}/[0-9]{4,}-[a-z0-9-]+\.md$`)

// CorpusMergeCheck fails if a PR introduces a record id that already exists on
// base at another path, or if two introduced record files share an id.
func CorpusMergeCheck(ctx context.Context, p MergeParams) error {
	if err := p.validate(); err != nil {
		return err
	}
	baseIDs, err := idsAtRef(ctx, p.Corpus, p.Base)
	if err != nil {
		return fmt.Errorf("loading base ids: %w", err)
	}
	changed, err := changedPaths(ctx, p)
	if err != nil {
		return err
	}
	seenHead := make(map[string]string)
	var errs []error
	for _, path := range changed {
		if !isRecordPath(path) {
			continue
		}
		rec, err := recordAtRef(ctx, p.Corpus, p.Head, path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		if first, ok := seenHead[rec.ID]; ok && first != path {
			errs = append(errs, fmt.Errorf("duplicate introduced id %s in %s and %s", rec.ID, first, path))
		}
		seenHead[rec.ID] = path
		if basePath, ok := baseIDs[rec.ID]; ok && basePath != path {
			errs = append(errs, fmt.Errorf("introduced id %s in %s already exists on base in %s", rec.ID, path, basePath))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// CorpusPRPaths enforces the corpus PR path allowlist.
func CorpusPRPaths(ctx context.Context, p MergeParams) error {
	if err := p.validate(); err != nil {
		return err
	}
	changed, err := changedPaths(ctx, p)
	if err != nil {
		return err
	}
	var errs []error
	for _, path := range changed {
		if bad, why := forbiddenPRPath(path); bad {
			errs = append(errs, fmt.Errorf("path %s is not allowed in corpus PRs (%s)", path, why))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (p MergeParams) validate() error {
	if p.Corpus == "" {
		return errors.New("corpus is required")
	}
	if p.Base == "" {
		return errors.New("base ref is required")
	}
	if p.Head == "" {
		return errors.New("head ref is required")
	}
	return nil
}

func changedPaths(ctx context.Context, p MergeParams) ([]string, error) {
	out, err := git(ctx, p.Corpus, "diff", "--name-only", "--diff-filter=AM", p.Base+".."+p.Head)
	if err != nil {
		return nil, fmt.Errorf("diff %s..%s: %w", p.Base, p.Head, err)
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, filepath.ToSlash(line))
		}
	}
	return paths, nil
}

func idsAtRef(ctx context.Context, repo, ref string) (map[string]string, error) {
	out, err := git(ctx, repo, "ls-tree", "-r", "--name-only", ref, "--", "experience")
	if err != nil {
		return nil, err
	}
	ids := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		path := strings.TrimSpace(filepath.ToSlash(line))
		if path == "" || !isRecordPath(path) {
			continue
		}
		rec, err := recordAtRef(ctx, repo, ref, path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		ids[rec.ID] = path
	}
	return ids, nil
}

func recordAtRef(ctx context.Context, repo, ref, path string) (*record.Record, error) {
	src, err := git(ctx, repo, "show", ref+":"+path)
	if err != nil {
		return nil, err
	}
	return record.Parse(path, []byte(src))
}

func isRecordPath(path string) bool {
	return recordPathRE.MatchString(filepath.ToSlash(path))
}

// forbiddenRunsRE matches the FIXED-path operational files that must never enter a
// corpus PR: committing them collided every validate PR on a shared path (ADR-0027).
// Per-run files (runs/run-<id>-{promote,adapt}.json) are uniquely named and allowed.
var forbiddenRunsRE = []*regexp.Regexp{
	regexp.MustCompile(`^runs/[^/]+\.journal\.json$`), // promote/adapt resume cursors
	regexp.MustCompile(`^runs/[^/]+\.holds\.json$`),   // hold-cooldown ledger
}

// forbiddenPRPath is a DENYLIST: it rejects only the operational/generated files that
// cause cross-PR conflicts or don't belong in the data product. Everything else —
// records, per-run manifests, and ordinary repo files (.gitignore, README, docs,
// CI workflows) — is allowed, so legitimate corpus maintenance PRs are never blocked.
func forbiddenPRPath(path string) (bool, string) {
	path = filepath.ToSlash(path)
	for _, re := range forbiddenRunsRE {
		if re.MatchString(path) {
			return true, "fixed operational run-state (ADR-0027)"
		}
	}
	switch {
	case strings.HasSuffix(path, ".db"), strings.HasSuffix(path, ".db-wal"), strings.HasSuffix(path, ".db-shm"):
		return true, "generated database"
	case strings.HasSuffix(path, ".lock"):
		return true, "lockfile"
	}
	return false, ""
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("%v: %s", err, msg)
		}
		return "", err
	}
	return string(out), nil
}
