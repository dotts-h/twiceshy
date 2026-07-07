// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/screen"
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
	"A record is NEVER stored as validated here — human review on the PR is the trust boundary. " +
	"Set `redact_pii: true` to replace incidental low-severity PII (private IPs, emails) with placeholders " +
	"before recording so an incidental IP/email does not quarantine the draft; secrets are never redacted."

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
	RedactPII       bool     `json:"redact_pii,omitempty" jsonschema:"opt-in: replace incidental low-severity PII (private IPs, emails) with stable placeholders BEFORE recording, so an incidental IP/email does not quarantine the draft on a pii flag. Secrets are NEVER redacted."`
}

// RecordResult is the output of the record_experience tool.
type RecordResult struct {
	Novelty    string      `json:"novelty"`             // known | similar | novel
	RecordID   string      `json:"record_id,omitempty"` // allocated id (empty when known)
	Markdown   string      `json:"markdown,omitempty"`  // the quarantined record to PR (empty when known)
	Candidates []SearchHit `json:"candidates"`          // existing matches / leads (never nil; empty slice ok)
	Message    string      `json:"message"`
	Redacted   []string    `json:"redacted,omitempty"` // deduped "pii:rule" flags redacted (empty/omitted when none)
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

// validateAlphaRecordSize applies the tighter alpha-tenant caps (alpha_policy.go,
// ADR-0031) on top of validateRecordSize's engine-wide guardrails.
func validateAlphaRecordSize(args RecordArgs) error {
	if len(args.Body) > alphaMaxBodyBytes {
		return fmt.Errorf("body too large for an alpha tenant: %d bytes (max %d)", len(args.Body), alphaMaxBodyBytes)
	}
	if len(args.Summary) > alphaMaxSummaryBytes {
		return fmt.Errorf("summary too large for an alpha tenant: %d bytes (max %d)", len(args.Summary), alphaMaxSummaryBytes)
	}
	if n := len(args.ErrorSignatures); n > alphaMaxSignatures {
		return fmt.Errorf("too many error_signatures for an alpha tenant: %d (max %d)", n, alphaMaxSignatures)
	}
	for i, sig := range args.ErrorSignatures {
		if len(sig) > alphaMaxSignatureBytes {
			return fmt.Errorf("error_signatures[%d] too large for an alpha tenant: %d bytes (max %d)", i, len(sig), alphaMaxSignatureBytes)
		}
	}
	return nil
}

// recordScanTexts gathers every free-text field of RecordArgs the safety gate
// should see — the same field set redactRecordPII redacts — so the secret
// check below runs BEFORE a record is ever built or an id allocated.
func recordScanTexts(args RecordArgs) []string {
	return append([]string{args.Title, args.Summary, args.RootCause, args.Fix, args.GuardingTest, args.Body, args.Ecosystem, args.Package},
		args.ErrorSignatures...)
}

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

// redactRecordPII returns a copy of args with incidental low-severity PII
// (private IPs, emails) replaced by placeholders in every field the ingest gate
// scans, plus the deduped, sorted "pii:rule" flags it redacted. Secrets and
// harmful-code are never touched (screen.Redact is pii-only) — redaction cannot
// launder a secret past the gate. Caller-side on purpose: the detector/ingest
// stay pure (ADR-0011).
func redactRecordPII(args RecordArgs) (RecordArgs, []string) {
	var findings []screen.Finding
	redact := func(text string) string {
		redacted, found := screen.Redact(text)
		findings = append(findings, found...)
		return redacted
	}

	args.Title = redact(args.Title)
	args.Summary = redact(args.Summary)
	args.ErrorSignatures = append([]string(nil), args.ErrorSignatures...)
	for i := range args.ErrorSignatures {
		args.ErrorSignatures[i] = redact(args.ErrorSignatures[i])
	}
	args.RootCause = redact(args.RootCause)
	args.Fix = redact(args.Fix)
	args.GuardingTest = redact(args.GuardingTest)
	args.Body = redact(args.Body)
	args.Ecosystem = redact(args.Ecosystem)
	args.Package = redact(args.Package)

	return args, screen.Flags(findings)
}

// record processes a record_experience tool call. It builds a draft from the
// provided arguments, runs dedup-at-ingest, and returns either a known-duplicate
// result or a quarantined draft ready to be PR'd. It does NOT write to disk.
func (h *handlers) record(ctx context.Context, _ *mcp.CallToolRequest, args RecordArgs) (*mcp.CallToolResult, RecordResult, error) {
	start := time.Now()
	const tool = "record_experience"

	tenant := TenantFromContext(ctx)
	alpha := isAlphaTenant(tenant)

	if err := validateRecordSize(args); err != nil {
		h.logToolError(tool, start, err)
		return nil, RecordResult{}, err
	}
	if alpha {
		if err := validateAlphaRecordSize(args); err != nil {
			h.logToolError(tool, start, err)
			return nil, RecordResult{}, err
		}
		if err := h.checkContributionQuota(ctx, tool, alphaContributionQuotas[tool]); err != nil {
			h.logToolError(tool, start, err)
			return nil, RecordResult{}, err
		}
	}

	// ADR-0030 phase 2 (#0128): an alpha tenant's redaction is FORCED on
	// regardless of the caller-supplied arg — every submission is hostile
	// until proven otherwise.
	var redactedFlags []string
	if args.RedactPII || alpha {
		args, redactedFlags = redactRecordPII(args)
	}

	// Secret-shaped content is never redacted and, for an untrusted alpha
	// tenant, never quarantined either: fail-closed, reject outright before a
	// record is built or an id allocated — nothing lands on disk. (Operator
	// behavior is unchanged: a secret from the trusted channel still just
	// quarantines with a security_flags entry, as before.)
	if alpha {
		// session is caller-supplied free text that gets persisted
		// (provenance.session) but sits outside recordScanTexts' field set
		// (that helper's doc comment ties it to redactRecordPII's fields) —
		// append it here so the fail-closed scan still covers it.
		if err := rejectAlphaSecrets(tool, append(recordScanTexts(args), args.Session)...); err != nil {
			h.logToolError(tool, start, err)
			return nil, RecordResult{}, err
		}
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

	// TENANT ORIGIN STAMPING (#0128, ADR-0030 phase 2): a tok_ tenant's draft
	// provenance origin/author is FORCED to "alpha:<token_id>" — the trust key
	// the push-eligibility gate keys off. A caller-supplied author is never
	// allowed to spoof an importer/trusted origin; it is preserved only as a
	// display note in the narrative body, never as provenance.source.author.
	recordAuthor := args.Author
	if alpha {
		var display string
		recordAuthor, display = alphaStampAuthor(tenant, args.Author)
		if display != "" {
			draft.Body = fmt.Sprintf("_Submitted as: %s (untrusted alpha tenant; recorded origin: %s)_\n\n%s",
				display, recordAuthor, draft.Body)
		}
	}

	// Allocate a new ID against the source-of-truth corpus, robust to a live index
	// that has drifted behind the committed records (#0059) AND to a prior allocation
	// in this same server process (#0089 — two calls in one session must not collide).
	id, err := h.allocateNextID(ctx)
	if err != nil {
		h.logToolError(tool, start, err)
		return nil, RecordResult{}, err
	}

	// Build metadata.
	meta := ingest.Meta{
		ID:     id,
		Author: recordAuthor,
		Now:    time.Now().UTC().Format("2006-01-02"),
	}
	if args.Session != "" {
		s := args.Session
		meta.Session = &s
	}

	// Run the ingest pipeline.
	out, err := ingest.Prepare(ctx, h.ix, h.repo, draft, meta)
	if err != nil {
		h.logToolError(tool, start, err)
		return nil, RecordResult{}, err
	}

	// Map candidates to SearchHit.
	cands := make([]SearchHit, len(out.Candidates))
	for i, c := range out.Candidates {
		cands[i] = SearchHit{
			ID:      c.ID,
			Kind:    c.Kind,
			Status:  c.Status,
			Title:   capText(sanitizeForTransport(c.Title), maxSearchTitleBytes),
			Summary: capText(sanitizeForTransport(c.Summary), maxSearchSummaryBytes),
			Score:   c.Score,
			Matched: c.Matched,
		}
	}

	// Handle known duplicates.
	if out.Novelty == index.NoveltyKnown {
		result := RecordResult{
			Novelty:    string(out.Novelty),
			Candidates: cands,
			Message:    "Already recorded — see the existing record in candidates; nothing was created.",
			Redacted:   redactedFlags,
		}
		h.logRecordOK(tool, start, result)
		return nil, result, nil
	}

	// Marshal the record for similar/novel cases.
	md, err := record.Marshal(out.Record)
	if err != nil {
		h.logToolError(tool, start, err)
		return nil, RecordResult{}, err
	}

	msg := "Quarantined draft created — open it as a PR to validate; it is NOT yet active."
	if flags := out.Record.Provenance.SecurityFlags; len(flags) > 0 {
		msg += " SECURITY: the safety gate flagged this draft (" + strings.Join(flags, ", ") +
			"); it cannot be promoted to validated until the hazard is resolved."
	}
	if len(redactedFlags) > 0 {
		msg += " Redacted incidental PII (" + strings.Join(redactedFlags, ", ") + ") before recording."
	}

	result := RecordResult{
		Novelty:    string(out.Novelty),
		RecordID:   out.Record.ID,
		Markdown:   string(md),
		Candidates: cands,
		Message:    msg,
		Redacted:   redactedFlags,
	}
	h.logRecordOK(tool, start, result)
	return nil, result, nil
}

func (h *handlers) logRecordOK(tool string, start time.Time, result RecordResult) {
	h.logger.Info("tool call",
		slog.String("tool", tool),
		slog.String("outcome", "ok"),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		slog.String("novelty", result.Novelty),
		slog.String("record_id", result.RecordID),
	)
}
