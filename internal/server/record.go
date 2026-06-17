// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// recordDescription is the load-bearing tool description that tells the model
// when to call the record_experience tool, what arguments to pass, and that
// a known-duplicate result is a useful answer.
const recordDescription = "Propose a new engineering-experience record after solving a novel trap, bug, " +
	"or dead-end worth remembering so the next agent avoids it. " +
	"Pass `kind` (trap|fix|dead-end|convention|workflow), `title`, a symptom `summary`, the verbatim " +
	"`error_signatures`, `ecosystem`/`package`, `root_cause`, `fix`, the `guarding_test`, a markdown `body`, " +
	"and `author`. " +
	"The result's `novelty` is one of: `known` — an existing record already covers this (its id is in " +
	"`candidates`); nothing is created. `similar`/`novel` — a quarantined draft is returned in `markdown` " +
	"with an allocated `record_id`; open it as a pull request to validate it. " +
	"A record is NEVER stored as validated here — human review on the PR is the trust boundary."

// RecordArgs is the input to the record_experience tool.
type RecordArgs struct {
	Kind            string   `json:"kind" jsonschema:"trap|fix|dead-end|convention|workflow"`
	Title           string   `json:"title" jsonschema:"one-line title"`
	Summary         string   `json:"summary,omitempty" jsonschema:"short symptom summary"`
	ErrorSignatures []string `json:"error_signatures,omitempty" jsonschema:"verbatim error lines"`
	Ecosystem       string   `json:"ecosystem,omitempty"`
	Package         string   `json:"package,omitempty"`
	RootCause       string   `json:"root_cause,omitempty"`
	Fix             string   `json:"fix,omitempty"`
	GuardingTest    string   `json:"guarding_test,omitempty"`
	Body            string   `json:"body" jsonschema:"markdown narrative"`
	Author          string   `json:"author" jsonschema:"who is proposing this"`
	Session         string   `json:"session,omitempty"`
}

// RecordResult is the output of the record_experience tool.
type RecordResult struct {
	Novelty    string      `json:"novelty"`             // known | similar | novel
	RecordID   string      `json:"record_id,omitempty"` // allocated id (empty when known)
	Markdown   string      `json:"markdown,omitempty"`  // the quarantined record to PR (empty when known)
	Candidates []SearchHit `json:"candidates"`          // existing matches / leads (never nil; empty slice ok)
	Message    string      `json:"message"`
}

// Input-size caps for record_experience. The channel is bearer-authed
// (trusted), so these are guardrails, not a security boundary — but one call
// shouldn't drive unbounded work, and each error signature costs a dedup probe.
const (
	maxRecordBodyBytes  = 64 << 10 // 64 KiB markdown body
	maxRecordTitleBytes = 1 << 10  // record.validate further bounds title to 8..120 runes
	maxRecordSignatures = 32       // each drives a fingerprint dedup probe in ingest.Prepare
	maxSignatureBytes   = 4 << 10  // each signature is hashed and FTS-indexed
)

// validateRecordSize rejects oversized inputs cheaply, before NextID allocation
// and the per-signature dedup probes.
func validateRecordSize(args RecordArgs) error {
	if len(args.Body) > maxRecordBodyBytes {
		return fmt.Errorf("body too large: %d bytes (max %d)", len(args.Body), maxRecordBodyBytes)
	}
	if len(args.Title) > maxRecordTitleBytes {
		return fmt.Errorf("title too large: %d bytes (max %d)", len(args.Title), maxRecordTitleBytes)
	}
	if n := len(args.ErrorSignatures); n > maxRecordSignatures {
		return fmt.Errorf("too many error_signatures: %d (max %d)", n, maxRecordSignatures)
	}
	for i, sig := range args.ErrorSignatures {
		if len(sig) > maxSignatureBytes {
			return fmt.Errorf("error_signatures[%d] too large: %d bytes (max %d)", i, len(sig), maxSignatureBytes)
		}
	}
	return nil
}

// record processes a record_experience tool call. It builds a draft from the
// provided arguments, runs dedup-at-ingest, and returns either a known-duplicate
// result or a quarantined draft ready to be PR'd. It does NOT write to disk.
func (h *handlers) record(ctx context.Context, _ *mcp.CallToolRequest, args RecordArgs) (*mcp.CallToolResult, RecordResult, error) {
	if err := validateRecordSize(args); err != nil {
		return nil, RecordResult{}, err
	}

	// Build the draft from args.
	draft := ingest.Draft{
		Kind:  args.Kind,
		Title: args.Title,
		Body:  args.Body,
	}

	if args.Summary != "" || len(args.ErrorSignatures) > 0 {
		draft.Symptom = &record.Symptom{
			Summary:         args.Summary,
			ErrorSignatures: args.ErrorSignatures,
		}
	}

	if args.Ecosystem != "" || args.Package != "" {
		draft.AppliesTo = []record.AppliesTo{{
			Ecosystem: args.Ecosystem,
			Package:   args.Package,
		}}
	}

	if args.RootCause != "" || args.Fix != "" {
		draft.Resolution = &record.Resolution{
			RootCause: args.RootCause,
			Fix:       args.Fix,
		}
	}

	if args.GuardingTest != "" {
		gt := args.GuardingTest
		draft.Guard = &record.Guard{
			GuardingTest: &gt,
		}
	}

	// Allocate a new ID.
	id, err := h.ix.NextID(ctx)
	if err != nil {
		return nil, RecordResult{}, err
	}

	// Build metadata.
	meta := ingest.Meta{
		ID:     id,
		Author: args.Author,
		Now:    time.Now().UTC().Format("2006-01-02"),
	}
	if args.Session != "" {
		s := args.Session
		meta.Session = &s
	}

	// Run the ingest pipeline.
	out, err := ingest.Prepare(ctx, h.ix, h.repo, draft, meta)
	if err != nil {
		return nil, RecordResult{}, err
	}

	// Map candidates to SearchHit.
	cands := make([]SearchHit, len(out.Candidates))
	for i, c := range out.Candidates {
		cands[i] = SearchHit{
			ID:      c.ID,
			Kind:    c.Kind,
			Status:  c.Status,
			Title:   c.Title,
			Summary: c.Summary,
			Score:   c.Score,
			Matched: c.Matched,
		}
	}

	// Handle known duplicates.
	if out.Novelty == index.NoveltyKnown {
		return nil, RecordResult{
			Novelty:    string(out.Novelty),
			Candidates: cands,
			Message:    "Already recorded — see the existing record in candidates; nothing was created.",
		}, nil
	}

	// Marshal the record for similar/novel cases.
	md, err := record.Marshal(out.Record)
	if err != nil {
		return nil, RecordResult{}, err
	}

	return nil, RecordResult{
		Novelty:    string(out.Novelty),
		RecordID:   out.Record.ID,
		Markdown:   string(md),
		Candidates: cands,
		Message:    "Quarantined draft created — open it as a PR to validate; it is NOT yet active.",
	}, nil
}
