// SPDX-License-Identifier: AGPL-3.0-only

// Package eval is twiceshy's retrieval-effectiveness eval (#0005, ADR-0001 §8 /
// ADR-0011 §6): the evidence gate for the whole store. It answers the necessary
// precondition for any value claim — when an agent hits the situation a trap
// record describes, does the store surface THAT record in the top-k pull result,
// without drowning it in near-miss noise?
//
// This is the cheap, deterministic first slice (no agent, no LLM budget): it
// drives the same `search_experience` pull path an agent uses, with queries taken
// from each record's own error signatures (the text an agent actually sees) and
// symptom summary. The full agent-task with/without-retrieval eval (does the card
// change task success) is the follow-on slice.
//
// Limitation it is honest about: queries are derived from the records, so a hit
// partly reflects that a record indexes its own signature. The signal that still
// matters is comparative — recall@k below 1.0 means realistic phrasings miss, and
// near-miss rate measures whether the wrong card surfaces for a given symptom.
package eval

import (
	"context"
	"strings"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// Case is one retrieval probe: a realistic query an agent would issue, and the
// record we expect the store to surface for it.
type Case struct {
	RecordID string
	Query    string
	Source   string // "error_signature" | "summary" — where the query came from
}

// Searcher is the retrieval seam the eval drives (satisfied by *index.Index).
type Searcher interface {
	Search(ctx context.Context, q index.Query) ([]index.Hit, error)
}

// CaseResult is one probe's outcome.
type CaseResult struct {
	Case
	Found    bool     // the expected record was in the top-k
	Rank     int      // 1-based rank of the expected record; 0 if absent
	Returned []string // all record ids returned, in order (for near-miss inspection)
}

// NearMiss reports whether the top hit was a DIFFERENT record than expected — a
// wrong card surfacing ahead of (or instead of) the right one.
func (r CaseResult) NearMiss() bool {
	return len(r.Returned) > 0 && r.Returned[0] != r.RecordID
}

// Report is the aggregate over all cases.
type Report struct {
	K            int
	Cases        int
	Found        int
	RecallAtK    float64 // fraction of cases whose expected record was in top-k
	MRR          float64 // mean reciprocal rank of the expected record
	NearMissRate float64 // fraction of cases whose top hit was the wrong record
	Results      []CaseResult
}

// Cases derives eval probes from the behavioral records (trap / fix / dead-end)
// that carry retrievable symptoms. Each error signature becomes its own probe
// (the verbatim text an agent sees), plus the symptom summary as a paraphrase
// probe. Only VALIDATED records contribute: the default pull path serves
// validated-only, so a quarantined record is unretrievable by design and would
// be a meaningless miss — the eval measures the affordance an agent actually has.
// Records without a symptom contribute nothing.
func Cases(recs []*record.Record) []Case {
	var cases []Case
	for _, r := range recs {
		if r.Symptom == nil || r.Status != "validated" {
			continue
		}
		switch r.Kind {
		case "trap", "fix", "dead-end":
		default:
			continue
		}
		for _, sig := range r.Symptom.ErrorSignatures {
			if s := strings.TrimSpace(sig); s != "" {
				cases = append(cases, Case{RecordID: r.ID, Query: s, Source: "error_signature"})
			}
		}
		if s := strings.TrimSpace(r.Symptom.Summary); s != "" {
			cases = append(cases, Case{RecordID: r.ID, Query: s, Source: "summary"})
		}
	}
	return cases
}

// Run executes every case against the searcher (validated-only, the default pull
// path) and aggregates the report. k is clamped by the index to MaxK.
func Run(ctx context.Context, s Searcher, cases []Case, k int) (Report, error) {
	rep := Report{K: k, Cases: len(cases)}
	var rrSum float64
	var nearMiss int
	for _, c := range cases {
		hits, err := s.Search(ctx, index.Query{Text: c.Query, K: k})
		if err != nil {
			return Report{}, err
		}
		res := CaseResult{Case: c}
		for i, h := range hits {
			res.Returned = append(res.Returned, h.ID)
			if h.ID == c.RecordID && res.Rank == 0 {
				res.Found = true
				res.Rank = i + 1
			}
		}
		if res.Found {
			rep.Found++
			rrSum += 1.0 / float64(res.Rank)
		}
		if res.NearMiss() {
			nearMiss++
		}
		rep.Results = append(rep.Results, res)
	}
	if rep.Cases > 0 {
		rep.RecallAtK = float64(rep.Found) / float64(rep.Cases)
		rep.MRR = rrSum / float64(rep.Cases)
		rep.NearMissRate = float64(nearMiss) / float64(rep.Cases)
	}
	return rep, nil
}
