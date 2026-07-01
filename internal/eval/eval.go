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

// --- Push-channel precision eval (#0005, the push half / ADR-0001 §4) ---
//
// The pull eval above measures recall — does the right card surface. The push
// channel's binding failure mode is the opposite: PRECISION. Push injects
// unprompted on every prompt, so an off-domain prompt that surfaces ANY card is
// pure noise (the "near-zero value" the per-prompt hook produced — a Svelte +
// FastAPI session drowning in Go/SQLite/MCP cards). This is the measurement that
// was missing: off-domain prompts MUST inject zero cards, while genuine in-domain
// error queries MUST still surface their trap. It is the gate that justifies the
// push channel being on at all (CLAUDE.md: don't re-enable push without it).

// PushCase is one push probe. A non-empty ExpectID is a POSITIVE — a genuine
// query whose trap must be injected. An empty ExpectID is a NEGATIVE — an
// off-domain prompt that must inject nothing (the precision assertion).
type PushCase struct {
	Query    string
	ExpectID string // "" = negative (expect zero injection)
	Note     string
}

// Pusher is the push-retrieval seam the eval drives (satisfied by *index.Index).
type Pusher interface {
	RetrievePush(ctx context.Context, q index.Query) ([]index.Hit, error)
}

// PushNegatives are realistic off-domain prompts — the Svelte / FastAPI / recipe /
// prose sessions that drowned in irrelevant cards. Each is adversarial: it carries
// common dev vocabulary (http, method, request, handler, status, version, cache,
// permission, recipe) that the document-frequency gate mistook for a discriminative
// signal because those words are merely rare in a tiny curated corpus, not specific.
// Every one MUST inject zero cards.
func PushNegatives() []PushCase {
	return []PushCase{
		{Query: "make the svelte component re-render when the writable store changes", Note: "frontend"},
		{Query: "parse the fastapi request body and validate the pydantic transcript model", Note: "backend"},
		{Query: "close out the recipe and bump the changelog version for this release", Note: "recipe+version"},
		{Query: "the http handler returns the wrong status code for this request method", Note: "http+handler+method+request"},
		{Query: "add a cache layer and a permission check to the user settings page", Note: "cache+permission"},
		{Query: "refactor the react props drilling into a context provider with hooks", Note: "frontend"},
		{Query: "update the kubernetes deployment replicas then roll out the new image", Note: "ops"},
		{Query: "what is a good birthday gift to buy for my mother this year", Note: "pure prose"},
		// Adversarial sentences that the first stoplist pass still leaked (a reviewer
		// found "build"/"data"/"function"/"value"/"fails" et al. unlisted) — these are
		// the honest threat model: realistic off-domain prose, not words fitted to the list.
		{Query: "my react component re-renders on every keystroke and the build is slow", Note: "build"},
		{Query: "the python function returns the wrong value for an empty data file", Note: "function/value/data/file"},
		{Query: "write a bash script to read the config file and start the service", Note: "read/config/file/service"},
		{Query: "my unit test for the date parser fails on a leap year", Note: "unit/date/fails"},
		{Query: "deploy to aws with terraform and check the ci pipeline logs", Note: "aws/terraform/ci"},
		{Query: "add a graphql endpoint and a redis cache behind the nginx proxy", Note: "graphql/redis/nginx"},
		{Query: "the css grid layout breaks on mobile when a large image loads", Note: "css/layout/mobile"},
		// The reproduced live specimen (#0106/ADR-0028): a deep-analysis meta-prompt
		// whose only discriminative tokens ("application", "llm") each lived in a
		// DIFFERENT unrelated validated record — the cross-record false-positive the
		// #0108 corroboration rule exists to close.
		{Query: "need a deep analysis of this application and why it is still not working well not helping any llm", Note: "specimen: cross-record disc tokens, #0106"},
	}
}

// PushPositives are genuine in-domain error queries that MUST still surface their
// trap. Each carries at least two specific, co-occurring tokens (fts5+syntax,
// fts5+bm25, servemux+catch-all, fork/exec+tmpdir/noexec, rand.Seed+staticcheck,
// setup-go+forgejo/runner) — not common dev vocabulary — because a prompt-triggered
// push now requires TWO independent discriminative tokens landing on the SAME
// eligible record (#0108's corroboration rule), not one. Ids are the validated
// engineering traps (the original 9).
func PushPositives() []PushCase {
	return []PushCase{
		{Query: `fts5 match throws a syntax error near "." when the query has a dotted module path`, ExpectID: "exp-0001"},
		{Query: "fts5 bm25 scores are negative so order by rank desc returns the worst rows", ExpectID: "exp-0002"},
		{Query: "go servemux POST pattern lets other methods fall through to the catch-all not a 405", ExpectID: "exp-0006"},
		{Query: "go test fork/exec permission denied because TMPDIR is a noexec mount", ExpectID: "exp-0017"},
		{Query: "rand.Seed is deprecated since go 1.20, staticcheck flags the global source", ExpectID: "exp-0045"},
		{Query: "actions/setup-go cache step hangs for five minutes on a self-hosted forgejo runner", ExpectID: "exp-0005"},
	}
}

// PushReport aggregates a push precision/recall run.
type PushReport struct {
	Negatives       int
	FalseInjections int // negatives that injected >=1 card — MUST be 0
	Positives       int
	Recalled        int      // positives whose expected card was injected
	Leaks           []string // "query -> [ids]" for each false injection
	Misses          []string // "query (want id, got [...])" for each unrecalled positive
}

// Precision is 1 - falseInjectionRate over the negatives (1.0 = no off-domain noise).
func (r PushReport) Precision() float64 {
	if r.Negatives == 0 {
		return 1
	}
	return 1 - float64(r.FalseInjections)/float64(r.Negatives)
}

// Recall is the fraction of positives whose trap was injected.
func (r PushReport) Recall() float64 {
	if r.Positives == 0 {
		return 1
	}
	return float64(r.Recalled) / float64(r.Positives)
}

// RunPush drives the push gate (RetrievePush, with its discriminative-token
// precondition) over the precision/recall set and aggregates the report.
func RunPush(ctx context.Context, p Pusher, cases []PushCase) (PushReport, error) {
	var rep PushReport
	for _, c := range cases {
		hits, err := p.RetrievePush(ctx, index.Query{Text: c.Query})
		if err != nil {
			return PushReport{}, err
		}
		ids := make([]string, 0, len(hits))
		for _, h := range hits {
			ids = append(ids, h.ID)
		}
		if c.ExpectID == "" { // negative: must inject nothing
			rep.Negatives++
			if len(hits) > 0 {
				rep.FalseInjections++
				rep.Leaks = append(rep.Leaks, c.Query+" -> ["+strings.Join(ids, " ")+"]")
			}
			continue
		}
		rep.Positives++
		found := false
		for _, id := range ids {
			if id == c.ExpectID {
				found = true
				break
			}
		}
		if found {
			rep.Recalled++
		} else {
			rep.Misses = append(rep.Misses, c.Query+" (want "+c.ExpectID+", got ["+strings.Join(ids, " ")+"])")
		}
	}
	return rep, nil
}
