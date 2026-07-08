// SPDX-License-Identifier: AGPL-3.0-only

// prospect.go implements the model-hard trap prospector (ADR-0029, #0113): for
// each eligible corpus record it drafts a coding task an unwarned coder would
// answer by walking into the trap, runs the base model WITHOUT the card (the
// OFF arm), and executably verifies the output. A miss (the trap bit) triggers
// an ON-arm run WITH the card, measuring whether the card actually helps — the
// "on-also-fails" outcome is a distinct, visible lead (#0114). Corpus records are
// never mutated; this only reads them.
package agenteval

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/similarity"
)

// ErrTaskUnsupported means a TaskDrafter declined a record — not a hard error;
// the prospector counts it as a skip and continues, mirroring
// internal/drafter.ErrUnsupported's "decline, don't abort" contract.
var ErrTaskUnsupported = errors.New("agenteval: record not covered by this task drafter")

// TaskDrafter drafts a TaskCase from a corpus record: a natural, self-contained
// coding task that an unwarned coder would answer by hitting the record's trap.
// Returns ErrTaskUnsupported when the record is out of the drafter's class.
type TaskDrafter interface {
	Name() string
	DraftTask(ctx context.Context, rec *record.Record) (TaskCase, error)
}

// leakShingleThreshold is the word-shingle containment (internal/similarity,
// DefaultN=5) above which a drafted prompt is treated as having leaked the
// record's own resolution text — the OFF arm would then be handed the fix inside
// its own task, making a "miss" meaningless. Matches the containment level
// similarity's own near-verbatim test flags at (see
// TestAssessNearVerbatimFlags, internal/similarity/similarity_test.go) and the
// -threshold default `twiceshy similarity` uses for the same class of check
// (cmd/twiceshy/similarity.go).
const leakShingleThreshold = 0.15

// ProspectCase is one drafted task that bit the base model in the OFF arm and was
// re-run in the ON arm with the record's card.
type ProspectCase struct {
	TrapID    string
	Prompt    string
	VerifyID  string
	Deps      []string
	Card      string // the card the ON arm ran with — needed to replay this case (#0114 gold emission)
	OnAvoided bool
	TokensOff int
	TokensOn  int
}

// ProspectReport aggregates one prospecting run.
type ProspectReport struct {
	Scanned, Eligible, Drafted int
	// Skipped counts records that never reached a verdict, by reason: "ineligible"
	// (prospectEligible false), "unsupported" (drafter declined), "leak" (the leak
	// guard tripped), "control" (the drafted control did not verify as avoided).
	Skipped map[string]int
	// OffAvoided lists the TrapIDs whose OFF arm already avoided the trap — no
	// ON arm was run for these (nothing to measure).
	OffAvoided []string
	// ModelHard lists every case whose OFF arm hit the trap, including BOTH
	// classes: OnAvoided true (the card helps) and OnAvoided false (the "card
	// exists but doesn't help" on-also-fails lead, #0114's distinct class).
	ModelHard []ProspectCase
}

// ProspectConfig configures one Prospect run.
type ProspectConfig struct {
	Records  []*record.Record
	Runner   AgentRunner
	Verifier Verifier
	Drafter  TaskDrafter
	// Max bounds the number of eligible-and-drafted cases the run processes; 0
	// (or negative) means unbounded.
	Max int
}

// Prospect walks cfg.Records in order, drafting and running at most cfg.Max
// eligible cases. For each: eligibility → draft (skip on ErrTaskUnsupported,
// abort on any other drafter error) → leak guard → control check → OFF run →
// verify. The control check runs tc.Control through the same Verifier.Avoided
// used for the OFF/ON arms; a control that doesn't verify as avoided voids the
// case (Skipped["control"]) before it ever reaches an arm. An OFF avoidance ends
// the case; an OFF hit triggers an ON run (card rendered from the record) and
// both arms' verdicts are recorded. Runner/Verifier errors abort the run, like
// agenteval.Run.
func Prospect(ctx context.Context, cfg ProspectConfig) (ProspectReport, error) {
	rep := ProspectReport{Skipped: map[string]int{}}

	for _, rec := range cfg.Records {
		if cfg.Max > 0 && rep.Drafted >= cfg.Max {
			break
		}
		rep.Scanned++

		if !prospectEligible(rec) {
			rep.Skipped["ineligible"]++
			continue
		}
		rep.Eligible++

		tc, err := cfg.Drafter.DraftTask(ctx, rec)
		if err != nil {
			if errors.Is(err, ErrTaskUnsupported) {
				rep.Skipped["unsupported"]++
				continue
			}
			return ProspectReport{}, prospectErr("drafting task", rec.ID, err)
		}

		refText := rec.Resolution.RootCause + " " + rec.Resolution.Fix
		if similarity.Assess(tc.Prompt, refText, similarity.DefaultN).Flagged(leakShingleThreshold) {
			rep.Skipped["leak"]++
			continue
		}

		controlAvoided, err := cfg.Verifier.Avoided(ctx, tc, tc.Control)
		if err != nil {
			if errors.Is(err, ErrDepsUnavailable) {
				rep.Skipped["deps"]++
				continue
			}
			return ProspectReport{}, prospectErr("control verify", rec.ID, err)
		}
		if !controlAvoided {
			rep.Skipped["control"]++
			continue
		}
		rep.Drafted++

		off, err := cfg.Runner.Run(ctx, tc.Prompt, "")
		if err != nil {
			return ProspectReport{}, prospectErr("OFF run", rec.ID, err)
		}
		offAvoided, err := cfg.Verifier.Avoided(ctx, tc, off.Output)
		if err != nil {
			if errors.Is(err, ErrDepsUnavailable) {
				// A deps-skip at OFF time should not count toward Drafted. Since Drafted was
				// already incremented after control verify, we decrement it to keep report arithmetic coherent.
				rep.Drafted--
				rep.Skipped["deps"]++
				continue
			}
			return ProspectReport{}, prospectErr("OFF verify", rec.ID, err)
		}
		if offAvoided {
			rep.OffAvoided = append(rep.OffAvoided, rec.ID)
			continue
		}

		card := renderProspectCard(rec)
		on, err := cfg.Runner.Run(ctx, tc.Prompt, card)
		if err != nil {
			return ProspectReport{}, prospectErr("ON run", rec.ID, err)
		}
		onAvoided, err := cfg.Verifier.Avoided(ctx, tc, on.Output)
		if err != nil {
			if errors.Is(err, ErrDepsUnavailable) {
				// A deps-skip at ON time should not count toward Drafted. Since Drafted was
				// already incremented after control verify, we decrement it to keep report arithmetic coherent.
				rep.Drafted--
				rep.Skipped["deps"]++
				continue
			}
			return ProspectReport{}, prospectErr("ON verify", rec.ID, err)
		}
		rep.ModelHard = append(rep.ModelHard, ProspectCase{
			TrapID:    rec.ID,
			Prompt:    tc.Prompt,
			VerifyID:  tc.VerifyID,
			Deps:      tc.Deps,
			Card:      card,
			OnAvoided: onAvoided,
			TokensOff: off.Tokens,
			TokensOn:  on.Tokens,
		})
	}
	return rep, nil
}

// prospectErr wraps err with the "agenteval: <step> for <id>: <err>" shape every
// abort path in Prospect uses, so the six call sites don't repeat the format string.
func prospectErr(step, id string, err error) error {
	return fmt.Errorf("agenteval: %s for %s: %w", step, id, err)
}

// prospectEligible mirrors the push channel's eligibility predicate (ADR-0028):
// status validated AND kind ∈ {trap, fix} AND the source author is not the
// importer origin. Kept local to agenteval (no internal/index dependency) — the
// prospector works from already-loaded corpus records, not the FTS index.
func prospectEligible(rec *record.Record) bool {
	if rec.Status != "validated" {
		return false
	}
	if rec.Kind != "trap" && rec.Kind != "fix" {
		return false
	}
	return strings.ToLower(rec.Provenance.Source.Author) != "twiceshy-importer"
}

// renderProspectCard renders a minimal experience card for the ON arm: title,
// symptom summary, and the fix — a few lines, NOT the server's full card
// renderer (which formats retrieval metadata this harness never has).
func renderProspectCard(rec *record.Record) string {
	lines := []string{rec.Title}
	if rec.Symptom != nil && rec.Symptom.Summary != "" {
		lines = append(lines, rec.Symptom.Summary)
	}
	if rec.Resolution != nil && rec.Resolution.Fix != "" {
		lines = append(lines, rec.Resolution.Fix)
	}
	return strings.Join(lines, "\n")
}
