// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

// stringList is a repeatable string flag (-error, -dead-end).
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// runLearned captures one agent-authored lesson into the local corpus via the
// existing ingest.Prepare pipeline (#0094).
func runLearned(ctx context.Context, args []string, out io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("learned", flag.ContinueOnError)
	c := addCommonFlags(fs)
	base := fs.String("base", "", "base git ref for merge-safe id allocation")
	openPRs := fs.Bool("open-prs", false, "also allocate ids above records on open corpus PRs (Forgejo API, #0121)")
	kind := fs.String("kind", "trap", "record kind (trap|fix|dead-end|convention|workflow)")
	title := fs.String("title", "", "record title (required)")
	summary := fs.String("summary", "", "symptom summary")
	var errorsigs stringList
	fs.Var(&errorsigs, "error", "verbatim error signature (repeatable)")
	rootCause := fs.String("root-cause", "", "resolution root cause")
	fix := fs.String("fix", "", "resolution fix")
	var deadEnds stringList
	fs.Var(&deadEnds, "dead-end", "dead-end tried (repeatable)")
	verifiedBy := fs.String("verified-by", "", "guarding test description")
	ecosystem := fs.String("ecosystem", "", "applies_to ecosystem")
	pkg := fs.String("package", "", "applies_to package")
	body := fs.String("body", "", "markdown body (auto-composed when empty)")
	author := fs.String("author", "claude", "provenance author")
	session := fs.String("session", "", "provenance session id")
	stdout := fs.Bool("stdout", false, "print rendered markdown instead of writing")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" {
		return errors.New("learned: -title is required")
	}

	ix, _, err := buildIndex(ctx, c, false)
	if err != nil {
		return err
	}
	defer func() { _ = ix.Close() }()

	// learned writes a record straight to disk for the caller to PR, so it
	// takes the same merge-safe high-water sources (-base, -open-prs) as the
	// batch intakes — a bare NextID here reopens the #0121 collision.
	floors, err := openPRFloors(ctx, c.corpus, *openPRs, getenv)
	if err != nil {
		return fmt.Errorf("getting open PR floors: %w", err)
	}
	id, err := ingest.NextIDWithBase(ctx, ix, c.corpus, *base, floors...)
	if err != nil {
		return err
	}

	draft := ingest.Draft{
		Kind:  *kind,
		Title: *title,
		Body:  *body,
	}
	summaryText := *summary
	if summaryText == "" && len(errorsigs) > 0 {
		summaryText = *title
	}
	// A trap/fix record must carry a resolution to satisfy record.Validate, but a
	// permissive symptom-only capture may lack one. Fill HELD placeholders rather
	// than fabricated ones: a root cause whose leading word is "None" is held by
	// promote.HasSubstantiveRootCause (#0094), so an un-diagnosed draft stays
	// quarantined-and-held until a real diagnosis is added — it can never slip past
	// the pre-gate toward promotion. ("unknown" would have bypassed that gate.)
	rc, fx := *rootCause, *fix
	if (*kind == "trap" || *kind == "fix") && (rc == "" || fx == "") {
		if rc == "" {
			rc = "None — not yet diagnosed (captured symptom-only via `twiceshy learned`)"
		}
		if fx == "" {
			fx = "None — not yet fixed (captured symptom-only via `twiceshy learned`)"
		}
	}
	if summaryText != "" || len(errorsigs) > 0 {
		draft.Symptom = &record.Symptom{
			Summary:         summaryText,
			ErrorSignatures: errorsigs,
		}
	}
	if *ecosystem != "" || *pkg != "" {
		draft.AppliesTo = []record.AppliesTo{{
			Ecosystem: *ecosystem,
			Package:   *pkg,
		}}
	}
	if rc != "" || fx != "" || len(deadEnds) > 0 {
		res := &record.Resolution{
			RootCause: rc,
			Fix:       fx,
		}
		for _, de := range deadEnds {
			res.DeadEnds = append(res.DeadEnds, record.DeadEnd{Tried: de})
		}
		draft.Resolution = res
	}
	if *verifiedBy != "" {
		gt := *verifiedBy
		draft.Guard = &record.Guard{GuardingTest: &gt}
	}
	if strings.TrimSpace(draft.Body) == "" {
		draft.Body = composeLearnedBody(summaryText, errorsigs, *rootCause, *fix, *verifiedBy)
	}

	if *rootCause == "" || *fix == "" {
		_, _ = fmt.Fprintln(out, "warning: draft missing root-cause and/or fix — likely to be held by the promote gate")
	}

	meta := ingest.Meta{
		ID:                 id,
		Author:             *author,
		Now:                time.Now().UTC().Format("2006-01-02"),
		IncludeQuarantined: true,
	}
	if *session != "" {
		s := *session
		meta.Session = &s
	}

	outcome, err := ingest.Prepare(ctx, ix, c.repo, draft, meta)
	if err != nil {
		return err
	}

	if outcome.Record == nil {
		covered := "unknown"
		if len(outcome.Candidates) > 0 {
			covered = outcome.Candidates[0].ID
		}
		_, _ = fmt.Fprintf(out, "learned: already covered by %s\n", covered)
		return nil
	}

	if *stdout {
		md, err := record.Marshal(outcome.Record)
		if err != nil {
			return err
		}
		_, _ = out.Write(md)
		return nil
	}

	if err := writeRecord(c.corpus, outcome.Record); err != nil {
		return err
	}
	rec := outcome.Record
	_, _ = fmt.Fprintf(out, "learned: wrote %s (%s, %s)\n", rec.Path, rec.ID, outcome.Novelty)
	if outcome.Novelty == index.NoveltySimilar && len(outcome.Candidates) > 0 {
		ids := make([]string, len(outcome.Candidates))
		for i, c := range outcome.Candidates {
			ids[i] = c.ID
		}
		_, _ = fmt.Fprintf(out, "see also: %s\n", strings.Join(ids, ", "))
	}
	return nil
}

func composeLearnedBody(summary string, errorsigs []string, rootCause, fix, verifiedBy string) string {
	var b strings.Builder
	if summary != "" || len(errorsigs) > 0 {
		b.WriteString("## Symptom\n")
		if summary != "" {
			b.WriteString(summary)
			if len(errorsigs) > 0 {
				b.WriteByte('\n')
			}
		}
		for _, sig := range errorsigs {
			if s := strings.TrimSpace(sig); s != "" {
				b.WriteString("> ")
				b.WriteString(s)
				b.WriteByte('\n')
			}
		}
	}
	if rootCause != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("## Root cause\n")
		b.WriteString(rootCause)
		b.WriteByte('\n')
	}
	if fix != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("## Fix\n")
		b.WriteString(fix)
		b.WriteByte('\n')
	}
	if verifiedBy != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("## Verified by\n")
		b.WriteString(verifiedBy)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
