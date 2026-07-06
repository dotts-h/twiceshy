// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
)

func runToken(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: twiceshy token <issue|revoke|list> [flags]")
	}
	switch args[0] {
	case "issue":
		return runTokenIssue(ctx, args[1:], out, os.Stderr)
	case "revoke":
		return runTokenRevoke(ctx, args[1:], out)
	case "list":
		return runTokenList(ctx, args[1:], out)
	default:
		return fmt.Errorf("unknown token subcommand %q (want issue, revoke, or list)", args[0])
	}
}

func runTokenIssue(_ context.Context, args []string, out, note io.Writer) error {
	fs := flag.NewFlagSet("token issue", flag.ContinueOnError)
	indexPath := fs.String("index", "twiceshy.db", "path of the derived SQLite index")
	label := fs.String("label", "", "human-readable label for the token")
	dailyQuota := fs.Int("daily-quota", 1000, "calls per UTC day; 0 = unlimited")
	ratePerMin := fs.Int("rate-per-min", 60, "calls per minute; 0 = server default")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ix, err := index.Open(*indexPath)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	full, id, err := ix.IssueToken(*label, *dailyQuota, *ratePerMin, time.Now().UTC())
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, full)
	_, _ = fmt.Fprintf(note, "token %s issued — store the bearer value now; it cannot be retrieved again\n", id)
	return nil
}

func runTokenRevoke(_ context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("token revoke", flag.ContinueOnError)
	indexPath := fs.String("index", "twiceshy.db", "path of the derived SQLite index")
	id := fs.String("id", "", "token id (e.g. tok_ab12cd34)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("token revoke: -id is required")
	}
	ix, err := index.Open(*indexPath)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	if err := ix.RevokeToken(*id, time.Now().UTC()); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "revoked %s\n", *id)
	return nil
}

func runTokenList(_ context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("token list", flag.ContinueOnError)
	indexPath := fs.String("index", "twiceshy.db", "path of the derived SQLite index")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ix, err := index.Open(*indexPath)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	tokens, err := ix.ListTokens(time.Now().UTC())
	if err != nil {
		return err
	}
	for _, line := range formatTokenListLines(tokens) {
		_, _ = fmt.Fprintln(out, line)
	}
	return nil
}

func formatTokenListLines(tokens []index.TokenInfo) []string {
	lines := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		status := "active"
		if tok.RevokedAt != nil {
			status = "revoked"
		}
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s\tquota=%d\trate=%d\tcalls_today=%d",
			tok.ID, tok.Label, status, tok.DailyQuota, tok.RatePerMin, tok.CallsToday))
	}
	return lines
}
