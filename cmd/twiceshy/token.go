// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/entitlement"
	"github.com/dotts-h/twiceshy/internal/index"
)

func runToken(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	if len(args) == 0 {
		return errors.New("usage: twiceshy token <issue|revoke|list|assign|report> [flags]")
	}
	switch args[0] {
	case "issue":
		return runTokenIssue(ctx, args[1:], out, os.Stderr, teamPlansEnabled(getenv))
	case "revoke":
		return runTokenRevoke(ctx, args[1:], out)
	case "list":
		return runTokenList(ctx, args[1:], out)
	case "assign":
		if !teamPlansEnabled(getenv) {
			return teamPlansDisabledError()
		}
		return runTokenAssign(ctx, args[1:], out)
	case "report":
		if !teamPlansEnabled(getenv) {
			return teamPlansDisabledError()
		}
		return runTokenReport(ctx, args[1:], out)
	default:
		return fmt.Errorf("unknown token subcommand %q (want issue, revoke, list, assign, or report)", args[0])
	}
}

func teamPlansEnabled(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("TWICESHY_TEAM_PLANS"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func teamPlansDisabledError() error {
	return errors.New("team-plan features are disabled; set TWICESHY_TEAM_PLANS=1 to enable")
}

func runTokenIssue(_ context.Context, args []string, out, note io.Writer, teamPlans bool) error {
	fs := flag.NewFlagSet("token issue", flag.ContinueOnError)
	indexPath := fs.String("index", "twiceshy.db", "path of the derived SQLite index")
	label := fs.String("label", "", "human-readable label for the token")
	dailyQuota := fs.Int("daily-quota", 1000, "calls per UTC day; 0 = unlimited")
	ratePerMin := fs.Int("rate-per-min", 60, "calls per minute; 0 = server default")
	planName := fs.String("plan", "", "entitlement plan (community|pro|team|enterprise; feature-gated)")
	organizationID := fs.String("organization", "", "organization id (feature-gated)")
	workspaceID := fs.String("workspace", "", "workspace id (feature-gated)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	planned := *planName != "" || *organizationID != "" || *workspaceID != ""
	var plan entitlement.Plan
	if planned {
		if !teamPlans {
			return teamPlansDisabledError()
		}
		if *planName == "" || *organizationID == "" || *workspaceID == "" {
			return errors.New("token issue: -plan, -organization, and -workspace are required together")
		}
		var err error
		plan, err = entitlement.ParsePlan(*planName)
		if err != nil {
			return err
		}
	}
	ix, err := index.Open(*indexPath)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	var full, id string
	if planned {
		full, id, err = ix.IssuePlannedToken(*label, *organizationID, *workspaceID, plan, time.Now().UTC())
	} else {
		full, id, err = ix.IssueToken(*label, *dailyQuota, *ratePerMin, time.Now().UTC())
	}
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, full)
	_, _ = fmt.Fprintf(note, "token %s issued — store the bearer value now; it cannot be retrieved again\n", id)
	return nil
}

func runTokenAssign(_ context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("token assign", flag.ContinueOnError)
	indexPath := fs.String("index", "twiceshy.db", "path of the durable tenant registry")
	id := fs.String("id", "", "token id")
	planName := fs.String("plan", "", "entitlement plan")
	organizationID := fs.String("organization", "", "organization id")
	workspaceID := fs.String("workspace", "", "workspace id")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *id == "" || *planName == "" || *organizationID == "" || *workspaceID == "" {
		return errors.New("token assign: -id, -plan, -organization, and -workspace are required")
	}
	plan, err := entitlement.ParsePlan(*planName)
	if err != nil {
		return err
	}
	ix, err := index.Open(*indexPath)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()
	if err := ix.AssignTokenPlan(*id, *organizationID, *workspaceID, plan, time.Now().UTC()); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "assigned %s organization=%s workspace=%s plan=%s\n", *id, *organizationID, *workspaceID, plan)
	return nil
}

func runTokenReport(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("token report", flag.ContinueOnError)
	indexPath := fs.String("index", "twiceshy.db", "path of the durable tenant registry")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ix, err := index.Open(*indexPath)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()
	rows, err := ix.PlanReport(ctx)
	if err != nil {
		return err
	}
	for _, tok := range rows {
		_, _ = fmt.Fprintf(out, "%s\t%s\torganization=%s\tworkspace=%s\tplan=%s\tquota=%d\trate=%d\n", tok.ID, tok.Label, tok.OrganizationID, tok.WorkspaceID, tok.Plan, tok.DailyQuota, tok.RatePerMin)
	}
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
