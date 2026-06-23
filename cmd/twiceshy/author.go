// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dotts-h/twiceshy/internal/author"
)

// runAuthor pre-stages a §5-clean authored record plus its repro skeletons under
// -corpus (#0091) — the fill-in-the-blanks flow for docs/AUTHORING.md. It refuses
// to overwrite any existing file (all-or-nothing), so re-running can't clobber
// in-progress authoring. The provenance is pre-filled authored-internal: no
// source_url, the §5 sentinel — right by construction.
func runAuthor(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("author", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root to write the skeleton under")
	id := fs.String("id", "", "record id (exp-NNNN)")
	slug := fs.String("slug", "", "kebab-case filename slug")
	title := fs.String("title", "", "record title")
	kind := fs.String("kind", "trap", "record kind (trap|fix|dead-end|convention|workflow)")
	authorName := fs.String("author", "claude", "provenance.source.author")
	withNeg := fs.Bool("with-negative", false, "also scaffold a negative repro (for a documented dead-end)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	files, err := author.Scaffold(author.Params{
		ID: *id, Slug: *slug, Title: *title, Kind: *kind, Author: *authorName, WithNegative: *withNeg,
	}, time.Now())
	if err != nil {
		return err
	}

	// All-or-nothing: refuse if any target already exists, before writing the first.
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(*corpus, f.Path)); err == nil {
			return fmt.Errorf("author: %s already exists — refusing to overwrite", f.Path)
		}
	}
	for _, f := range files {
		full := filepath.Join(*corpus, f.Path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("author: %w", err)
		}
		mode := os.FileMode(0o644)
		if filepath.Ext(f.Path) == ".sh" {
			mode = 0o755
		}
		if err := os.WriteFile(full, []byte(f.Content), mode); err != nil {
			return fmt.Errorf("author: writing %s: %w", f.Path, err)
		}
		_, _ = fmt.Fprintln(out, "wrote", f.Path)
	}
	_, _ = fmt.Fprintln(out, "\nFill in the TODO placeholders in your own words, implement the repro(s), then:")
	_, _ = fmt.Fprintln(out, "  twiceshy doctor revalidate   # prove the repro holds")
	_, _ = fmt.Fprintln(out, "  twiceshy similarity ...      # check the prose vs any suspected source (ADR-0011 §5)")
	return nil
}
