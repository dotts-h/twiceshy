// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
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

func runNextID(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("nextid", flag.ContinueOnError)
	corpus := fs.String("corpus", ".", "corpus root")
	base := fs.String("base", "", "base git ref")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "twiceshy-nextid-*.db")
	if err != nil {
		return err
	}
	db := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(db) }()
	ix, err := index.Open(db)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()
	id, err := ingest.NextIDWithBase(ctx, ix, *corpus, *base)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, id)
	return nil
}
