// SPDX-License-Identifier: AGPL-3.0-only

// Package promote is the positive direction of ADR-0013's closed loop (#0029):
// for an execution-provable record, a holding broker attestation PLUS a PASS
// from the diverse-model judge flips `quarantined → validated` with no human
// approver — recording the attestation and the verdict in `provenance.promotion`
// as the git-committed audit trail. Everything short of (holding attestation AND
// judge approve) leaves the record quarantined: the gate is the trust anchor and
// the judge fails safe (ADR-0013 §1, §6).
package promote

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/doctor"
	"github.com/dotts-h/twiceshy/internal/judge"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// maxReproContentBytes caps how much of a repro script is fed to the judge, so a
// pathological repro can't blow up the prompt.
const maxReproContentBytes = 64 << 10

// Attestor produces holding attestations for records — the revalidate doctor's
// role (#0020). *repro.Revalidator satisfies it; tests stub it so the promoter
// runs with no broker / Docker.
type Attestor interface {
	RunWithAttestations(ctx context.Context, recs []*record.Record) (doctor.Report, []repro.Attestation, error)
}

// Promoter decides and applies an autonomous promotion.
type Promoter struct {
	attestor  Attestor
	judge     judge.Judge
	readRepro func(reproPath string) (string, error) // corpus-relative repro path → content
	now       func() string                          // validated_at date, "YYYY-MM-DD"
}

// Option configures a Promoter.
type Option func(*Promoter)

// WithReproReader overrides how a corpus-relative repro path is read (tests
// inject a fake; the default reads from the corpus root, path-safely).
func WithReproReader(f func(string) (string, error)) Option {
	return func(p *Promoter) { p.readRepro = f }
}

// WithClock injects the validated_at clock (tests pin it).
func WithClock(now func() string) Option { return func(p *Promoter) { p.now = now } }

// NewPromoter builds a Promoter. attestor proves the repro, j is the diverse
// judge, root is the corpus root used to resolve repro scripts for the judge.
func NewPromoter(attestor Attestor, j judge.Judge, root string, opts ...Option) *Promoter {
	p := &Promoter{
		attestor:  attestor,
		judge:     j,
		readRepro: func(rel string) (string, error) { return readReproFile(root, rel) },
		now:       todayUTC,
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Outcome is the result of a promotion attempt.
type Outcome struct {
	Promoted    bool
	Reason      string // why it was NOT promoted (eligibility, attestation, or verdict)
	Attestation repro.Attestation
	Verdict     judge.Verdict
}

func skip(reason string) (Outcome, error) { return Outcome{Reason: reason}, nil }

// todayUTC is the default clock for the audit dates (validated_at / valid.until).
func todayUTC() string { return time.Now().UTC().Format("2006-01-02") }

// Eligible reports whether a record is in the execution-provable class that can
// be auto-promoted, and if not, why. It checks only the positive direction's
// cheap preconditions — quarantined, not an outcome report, not safety-flagged,
// carries executable proof; the holding attestation and the judge verdict are
// Promote's job. The CLI's dry-run preview and Promote share this one predicate.
func Eligible(rec *record.Record) (bool, string) {
	switch {
	case rec.Status != "quarantined":
		return false, "not quarantined"
	case rec.Provenance.Disputes != nil:
		return false, "record is an outcome report (#0031), not a promotable lesson"
	case len(rec.Provenance.SecurityFlags) > 0:
		return false, "security-flagged record cannot be validated"
	case !record.HasPositiveRepro(rec):
		return false, "no executable proof — left for a human (ADR-0013 §5)"
	}
	return true, ""
}

// Promote attempts to flip a single quarantined record to validated. It mutates
// rec in place ONLY on a successful promotion (the caller then persists the
// delta); on any non-promotion path rec is left untouched. A hard error
// (attestor/broker failure, an invalid promoted record) is returned distinct
// from a fail-safe non-promotion, which returns (Outcome{Promoted:false}, nil).
func (p *Promoter) Promote(ctx context.Context, rec *record.Record) (Outcome, error) {
	// Eligibility — only the execution-provable class is auto-promotable; the
	// rest stay for a human (ADR-0013 §5). This short-circuits BEFORE the costly
	// attestation + judge calls.
	if ok, reason := Eligible(rec); !ok {
		return skip(reason)
	}

	// Proof — a holding attestation is the trust anchor. The judge is never even
	// consulted without it.
	_, atts, err := p.attestor.RunWithAttestations(ctx, []*record.Record{rec})
	if err != nil {
		return Outcome{}, fmt.Errorf("promote %s: attest: %w", rec.ID, err)
	}
	if len(atts) == 0 {
		return skip("no attestation produced")
	}
	att := atts[0]
	if !att.Holds || att.Inconclusive {
		return Outcome{Reason: "attestation does not hold — stays quarantined (fail-safe)", Attestation: att}, nil
	}

	// Judgement — the secondary filter. Any error/garble fails safe.
	repros, err := p.reproArtifacts(rec)
	if err != nil {
		return Outcome{}, fmt.Errorf("promote %s: read repros: %w", rec.ID, err)
	}
	verdict, err := p.judge.Judge(ctx, judge.Request{Record: rec, Attestation: att, Repros: repros})
	if err != nil {
		return Outcome{Reason: "judge unavailable — stays quarantined (fail-safe): " + err.Error(), Attestation: att}, nil
	}
	if !verdict.Approved() {
		return Outcome{Reason: "judge did not approve — stays quarantined", Attestation: att, Verdict: verdict}, nil
	}

	// Promote — mutate to validated with the audit trail, then validate; revert
	// on any invalidity so the caller never persists a half-promoted record.
	origStatus, origValidatedAt, origPromotion := rec.Status, rec.Provenance.ValidatedAt, rec.Provenance.Promotion
	validatedAt := p.now()
	rec.Status = "validated"
	rec.Provenance.ValidatedAt = &validatedAt
	rec.Provenance.Promotion = &record.Promotion{
		AttestedAt:      att.RanAt,
		ReproducedUnder: att.ReproducedUnder,
		JudgeModel:      verdict.Model,
		JudgeDecision:   string(verdict.Decision),
	}
	if err := record.Validate(rec); err != nil {
		rec.Status, rec.Provenance.ValidatedAt, rec.Provenance.Promotion = origStatus, origValidatedAt, origPromotion
		return Outcome{}, fmt.Errorf("promote %s: promoted record is invalid (not persisted): %w", rec.ID, err)
	}
	return Outcome{Promoted: true, Attestation: att, Verdict: verdict}, nil
}

// RepromoteEligible reports whether a record is a demoted execution-provable
// lesson that can be restored to validated, and if not, why. Only stale or
// disputed records with executable proof and no security flags qualify; the
// holding attestation and the judge verdict are Repromote's job.
func RepromoteEligible(rec *record.Record) (bool, string) {
	switch {
	case rec.Status != "stale" && rec.Status != "disputed":
		return false, "not a demoted record"
	case len(rec.Provenance.SecurityFlags) > 0:
		return false, "security-flagged record cannot be validated"
	case !record.HasPositiveRepro(rec):
		return false, "no executable proof — left for a human (ADR-0013 §5)"
	}
	return true, ""
}

// Repromote attempts to restore a demoted record to validated. It mutates rec
// in place ONLY on a successful re-promotion (the caller then persists the
// delta); on any non-promotion path rec is left untouched. A hard error
// (attestor/broker failure, an invalid re-promoted record) is returned
// distinct from a fail-safe non-promotion, which returns (Outcome{Promoted:false}, nil).
func (p *Promoter) Repromote(ctx context.Context, rec *record.Record) (Outcome, error) {
	if ok, reason := RepromoteEligible(rec); !ok {
		return skip(reason)
	}

	_, atts, err := p.attestor.RunWithAttestations(ctx, []*record.Record{rec})
	if err != nil {
		return Outcome{}, fmt.Errorf("repromote %s: attest: %w", rec.ID, err)
	}
	if len(atts) == 0 {
		return skip("no attestation produced")
	}
	att := atts[0]
	if !att.Holds || att.Inconclusive {
		return Outcome{Reason: "attestation does not hold — stays demoted (fail-safe)", Attestation: att}, nil
	}

	repros, err := p.reproArtifacts(rec)
	if err != nil {
		return Outcome{}, fmt.Errorf("repromote %s: read repros: %w", rec.ID, err)
	}
	verdict, err := p.judge.Judge(ctx, judge.Request{Record: rec, Attestation: att, Repros: repros})
	if err != nil {
		return Outcome{Reason: "judge unavailable — stays demoted (fail-safe): " + err.Error(), Attestation: att}, nil
	}
	if !verdict.Approved() {
		return Outcome{Reason: "judge did not approve — stays demoted", Attestation: att, Verdict: verdict}, nil
	}

	origStatus := rec.Status
	origUntil := rec.Provenance.Valid.Until
	origValidatedAt := rec.Provenance.ValidatedAt
	origDemotion := rec.Provenance.Demotion
	origPromotion := rec.Provenance.Promotion
	validatedAt := p.now()
	rec.Status = "validated"
	rec.Provenance.ValidatedAt = &validatedAt
	rec.Provenance.Valid.Until = nil
	rec.Provenance.Demotion = nil
	rec.Provenance.Promotion = &record.Promotion{
		AttestedAt:      att.RanAt,
		ReproducedUnder: att.ReproducedUnder,
		JudgeModel:      verdict.Model,
		JudgeDecision:   string(verdict.Decision),
	}
	if err := record.Validate(rec); err != nil {
		rec.Status = origStatus
		rec.Provenance.Valid.Until = origUntil
		rec.Provenance.ValidatedAt = origValidatedAt
		rec.Provenance.Demotion = origDemotion
		rec.Provenance.Promotion = origPromotion
		return Outcome{}, fmt.Errorf("repromote %s: re-promoted record is invalid (not persisted): %w", rec.ID, err)
	}
	return Outcome{Promoted: true, Attestation: att, Verdict: verdict}, nil
}

// reproArtifacts resolves a record's repro scripts to their contents for the
// judge (the proof body, not just the attestation's pass/fail).
func (p *Promoter) reproArtifacts(rec *record.Record) ([]judge.ReproArtifact, error) {
	if rec.Guard == nil {
		return nil, nil
	}
	var arts []judge.ReproArtifact
	add := func(rp, kind, label string) error {
		content, err := p.readRepro(rp)
		if err != nil {
			return fmt.Errorf("repro %s: %w", rp, err)
		}
		arts = append(arts, judge.ReproArtifact{Path: rp, Kind: kind, Label: label, Content: content})
		return nil
	}
	if rec.Guard.Repro != nil && *rec.Guard.Repro != "" {
		if err := add(*rec.Guard.Repro, "positive", ""); err != nil {
			return nil, err
		}
	}
	for _, rp := range rec.Guard.Repros {
		if err := add(rp.Path, rp.Kind, rp.Label); err != nil {
			return nil, err
		}
	}
	return arts, nil
}

// readReproFile reads a corpus-relative repro path's content, refusing any path
// that escapes the corpus root. A directory repro (its own deps, #0026) is
// represented by its required repro.sh. The read is size-capped.
func readReproFile(root, rel string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("repro path %q escapes the corpus root", rel)
	}
	abs := filepath.Join(root, clean)
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		abs = filepath.Join(abs, "repro.sh")
	}
	f, err := os.Open(abs) //nolint:gosec // abs is rooted at the corpus and escape-checked above
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	// io.ReadAll over a LimitReader reads up to the cap correctly — a plain
	// f.Read can short-read and truncate a near-cap script mid-stream.
	b, err := io.ReadAll(io.LimitReader(f, maxReproContentBytes))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
