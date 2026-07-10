// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/dotts-h/twiceshy/internal/mergecheck"
)

func runCorpusMergeCheck(ctx context.Context, args []string, out io.Writer) error {
	p, err := parseMergeParams("corpus-merge-check", args)
	if err != nil {
		return err
	}
	if err := mergecheck.CorpusMergeCheck(ctx, p); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "corpus merge check OK")
	return nil
}

func runCorpusPRPaths(ctx context.Context, args []string, out io.Writer) error {
	p, err := parseMergeParams("corpus-pr-paths", args)
	if err != nil {
		return err
	}
	if err := mergecheck.CorpusPRPaths(ctx, p); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "corpus PR paths OK")
	return nil
}

func parseMergeParams(name string, args []string) (mergecheck.MergeParams, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	var p mergecheck.MergeParams
	fs.StringVar(&p.Corpus, "corpus", ".", "corpus root")
	fs.StringVar(&p.Base, "base", "", "base git ref")
	fs.StringVar(&p.Head, "head", "", "head git ref")
	if err := parseFlags(fs, args); err != nil {
		return p, err
	}
	return p, nil
}

func runNextID(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("nextid", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root")
	base := fs.String("base", "", "base git ref")
	openPRs := fs.Bool("open-prs", false, "also allocate ids above records on open corpus PRs (Forgejo API, #0121)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	floors, err := openPRFloors(ctx, *corpus, *openPRs, getenv)
	if err != nil {
		return fmt.Errorf("getting open PR floors: %w", err)
	}
	id, err := nextIDForCorpus(ctx, *corpus, *base, floors...)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, id)
	return nil
}
