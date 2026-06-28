// SPDX-License-Identifier: AGPL-3.0-only

package eval

import (
	"context"
	"fmt"
	"strings"

	"github.com/dotts-h/twiceshy/internal/retro"
)

// UsageCase is one gold-labeled session: the transcript the judge reads, the cards that were
// SERVED in it, and the subset the agent ACTUALLY used (the gold label). Used must be a subset
// of Served.
type UsageCase struct {
	Name       string
	Transcript string
	Served     []string // ids served/pushed in the session
	Used       []string // gold: the served ids the agent actually applied
}

// UsageReport micro-averages judge accuracy over the gold cases (restricted to SERVED cards —
// a verdict for a never-served id is ignored, exactly as the live join ignores it).
type UsageReport struct {
	Cases      int
	TP         int      // judge said used AND gold-used
	FP         int      // judge said used but gold-ignored
	FN         int      // gold-used but judge said ignored (or no verdict)
	Mismatches []string // "case: id (judge=used gold=ignored)" etc., for inspection
}

// Precision is TP/(TP+FP); 1.0 when TP+FP==0.
func (r UsageReport) Precision() float64 {
	if r.TP+r.FP == 0 {
		return 1
	}
	return float64(r.TP) / float64(r.TP+r.FP)
}

// Recall is TP/(TP+FN); 1.0 when TP+FN==0.
func (r UsageReport) Recall() float64 {
	if r.TP+r.FN == 0 {
		return 1
	}
	return float64(r.TP) / float64(r.TP+r.FN)
}

// RunUsage runs the judge over each case and accumulates TP/FP/FN. Per case: judge the
// transcript, keep only verdicts whose id is in Served, treat Used==true as the judge's
// positive set; compare to the gold Used set. A judge error aborts (the caller surfaces it —
// the eval needs every case judged to be meaningful, unlike the best-effort production join).
func RunUsage(ctx context.Context, judge retro.UsageJudge, cases []UsageCase) (UsageReport, error) {
	rep := UsageReport{Cases: len(cases)}
	for _, c := range cases {
		verdicts, err := judge.JudgeUsage(ctx, c.Transcript)
		if err != nil {
			return UsageReport{}, err
		}
		served := make(map[string]bool, len(c.Served))
		for _, id := range c.Served {
			served[id] = true
		}
		goldUsed := make(map[string]bool, len(c.Used))
		for _, id := range c.Used {
			goldUsed[id] = true
		}
		judgeUsed := make(map[string]bool)
		for _, v := range verdicts {
			if !served[v.ID] {
				continue
			}
			if v.Used {
				judgeUsed[v.ID] = true
			}
		}
		for id := range served {
			jUsed := judgeUsed[id]
			gUsed := goldUsed[id]
			switch {
			case jUsed && gUsed:
				rep.TP++
			case jUsed && !gUsed:
				rep.FP++
				rep.Mismatches = append(rep.Mismatches,
					fmt.Sprintf("%s: %s (judge=used gold=ignored)", c.Name, id))
			case !jUsed && gUsed:
				rep.FN++
				rep.Mismatches = append(rep.Mismatches,
					fmt.Sprintf("%s: %s (judge=ignored gold=used)", c.Name, id))
			}
		}
	}
	return rep, nil
}

// UsageGold is the hand-labeled gold set. SYNTHETIC for now (no real-traffic telemetry yet —
// see the activation note in #0069); each transcript is written so the use/ignore is
// unambiguous, to measure the judge on clear cases first.
func UsageGold() []UsageCase {
	return []UsageCase{
		{
			Name:   "one-used-one-ignored",
			Served: []string{"exp-0001", "exp-0017"},
			Used:   []string{"exp-0001"},
			Transcript: strings.TrimSpace(`
User: search_experience returned exp-0001 and exp-0017 for my FTS5 syntax error.

Agent: The query fails with fts5: syntax error near "." because MATCH treats the dotted
module path modernc.org/sqlite as FTS5 query syntax, not a literal string.

Following exp-0001: I will tokenize the user input, strip punctuation that FTS5 rejects,
and rebuild the MATCH clause from safe bareword tokens instead of passing the raw dotted path.

Agent: Applied exp-0001 — split on non-alphanumerics, quoted each token, joined with OR.
The search now runs without the syntax error.

The session never mentions TMPDIR, noexec mounts, fork/exec, or exp-0017's sandbox workaround.
`),
		},
		{
			Name:   "both-used",
			Served: []string{"exp-0002", "exp-0006"},
			Used:   []string{"exp-0002", "exp-0006"},
			Transcript: strings.TrimSpace(`
User: Two bugs — bm25 ranking returns worst rows first, and GET /push hits the catch-all instead of 405.

Agent: exp-0002 explains FTS5 bm25() is negative and lower-is-better. I changed ORDER BY rank DESC
to ORDER BY rank ASC so the best matches surface first, and rewrote the relevance floor as
bm25(t) <= -floor per the card.

Agent: exp-0006 explains ServeMux "POST /push" does not 405 other methods — non-POST falls through
to "/". I re-registered "/push" without a method prefix and return http.StatusMethodNotAllowed for
non-POST inside the handler, exactly as exp-0006 recommends.

Both fixes landed with guarding tests; the transcript quotes both card ids and their fixes.
`),
		},
		{
			Name:   "none-used",
			Served: []string{"exp-0001"},
			Used:   nil,
			Transcript: strings.TrimSpace(`
User: The CSS grid layout breaks on mobile when a large hero image loads — unrelated to search.

Agent: I adjusted grid-template-rows, set min-height: 0 on the flex child, and lazy-loaded the
hero image. No FTS5 MATCH errors, no dotted module paths, no fts5 syntax errors, and no retrieval
of exp-0001's tokenization fix was needed or applied.

exp-0001 was pushed into context earlier but this is a frontend layout bug; the agent never
quoted or applied the FTS5 tokenization lesson.
`),
		},
	}
}
