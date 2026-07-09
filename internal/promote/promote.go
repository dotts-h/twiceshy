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
	"unicode"

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
	attestor      Attestor
	judge         judge.Judge
	advisoryPanel judge.Judge                                                   // ADR-0016: diverse panel for advisory-class records; nil = skip
	prosePanel    judge.Judge                                                   // ADR-0020: cross-family (gpt-oss+agy) panel for prose-class records; nil = skip
	stalenessGate func(ctx context.Context, rec *record.Record) *doctor.Finding // #0071: born-stale gate; nil = ungated
	readRepro     func(reproPath string) (string, error)                        // corpus-relative repro path → content
	now           func() string                                                 // validated_at date, "YYYY-MM-DD"
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

// WithAdvisoryPanel wires the diverse judge panel for advisory-class promotions
// (ADR-0016). When nil, advisory records skip with a human-left reason.
func WithAdvisoryPanel(j judge.Judge) Option {
	return func(p *Promoter) { p.advisoryPanel = j }
}

// WithProsePanel wires the cross-family panel for prose-class promotions (ADR-0020):
// a no-repro, no-source lesson is judged on its own coherence + safety, poison gating,
// by a panel that excludes the gemini free tier (privacy, ADR-0016 §5). When nil,
// prose records skip with a human-left reason.
func WithProsePanel(j judge.Judge) Option {
	return func(p *Promoter) { p.prosePanel = j }
}

// WithStalenessGate refuses to promote a record the staleness doctor would
// immediately flag — a born-stale advisory (an EOL runtime, or a valid.until
// already past) is not promote-worthy: it would be demoted on the very next
// staleness run and, while validated, trips the D2 guard that gates the validate
// PR (#0071, the promote-side companion to #302). *doctor.Staleness.WouldFlag
// satisfies this; nil leaves the advisory path ungated.
func WithStalenessGate(f func(context.Context, *record.Record) *doctor.Finding) Option {
	return func(p *Promoter) { p.stalenessGate = f }
}

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
	Unjudged    bool   // the judge produced NO verdict (transport/substrate failure, not a decline) — fail-safe quarantined, but must NOT start the hold cooldown (#0123).
	Reason      string // why it was NOT promoted (eligibility, attestation, or verdict)
	Attestation repro.Attestation
	Verdict     judge.Verdict
}

func skip(reason string) (Outcome, error) { return Outcome{Reason: reason}, nil }

// todayUTC is the default clock for the audit dates (validated_at / valid.until).
func todayUTC() string { return time.Now().UTC().Format("2006-01-02") }

// HasSubstantiveRootCause reports whether rec states an actual root cause.
// A record whose root_cause is empty or merely asserts there is none
// ("None", "N/A", "none - a design convention ...") is advice, not a
// promotable trap/fix (#0094).
func HasSubstantiveRootCause(rec *record.Record) bool {
	if rec.Resolution == nil {
		return false
	}
	// Strip surrounding whitespace and quote characters, then lowercase.
	s := strings.ToLower(strings.TrimSpace(strings.Trim(strings.TrimSpace(rec.Resolution.RootCause), `"'`)))
	if s == "" {
		return false
	}
	// Strip leading punctuation or quote characters before the keyword check
	// (handles variants like `"– None ..."` where dashes precede the keyword).
	s = strings.TrimLeft(s, `"'`+"–—-.,;: ")
	// Match "none"/"n/a" as the leading WORD, not as a prefix of a longer word —
	// "nonexistent", "nonempty", "non-blocking" are real root causes, not "none".
	word := s
	if i := strings.IndexFunc(s, func(r rune) bool { return !unicode.IsLetter(r) }); i >= 0 {
		word = s[:i]
	}
	return word != "none" && !strings.HasPrefix(s, "n/a")
}

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

// EligibleAdvisory reports whether a quarantined advisory-class record may be
// auto-promoted via the diverse judge panel (ADR-0016), without broker proof.
func EligibleAdvisory(rec *record.Record) (bool, string) {
	switch {
	case rec.Status != "quarantined":
		return false, "not quarantined"
	case rec.Provenance.Disputes != nil:
		return false, "record is an outcome report (#0031), not a promotable lesson"
	case len(rec.Provenance.SecurityFlags) > 0:
		return false, "security-flagged record cannot be validated"
	case !record.IsAdvisoryClass(rec):
		return false, "not advisory-class"
	}
	return true, ""
}

// EligibleProse reports whether a quarantined prose-class record may be auto-promoted
// via the cross-family panel (ADR-0020), without proof or a cited source. The
// content-screen is MANDATORY here (a security-flagged prose record is held — ADR-0020
// §2c), as are quarantined and not-an-outcome-report; the panel is Promote's job.
func EligibleProse(rec *record.Record) (bool, string) {
	switch {
	case rec.Status != "quarantined":
		return false, "not quarantined"
	case rec.Provenance.Disputes != nil:
		return false, "record is an outcome report (#0031), not a promotable lesson"
	case len(rec.Provenance.SecurityFlags) > 0:
		return false, "security-flagged record cannot be validated"
	case !record.IsProseClass(rec):
		return false, "not prose-class (carries a vuln id or a repro)"
	}
	return true, ""
}

// Promotable reports whether Promote should attempt this record (proof path, advisory
// panel, or prose panel).
func Promotable(rec *record.Record) (bool, string) {
	// Root-cause pre-gate (#0094): advice without a specific root cause is not
	// promotable — held cheaply before any eligibility check.
	if !HasSubstantiveRootCause(rec) {
		return false, "no substantive root_cause — held (advice, not a trap/fix) (#0094)"
	}
	if ok, _ := EligibleAdvisory(rec); ok {
		return true, ""
	}
	if ok, _ := EligibleProse(rec); ok {
		return true, ""
	}
	return Eligible(rec)
}

// Promote attempts to flip a single quarantined record to validated. It mutates
// rec in place ONLY on a successful promotion (the caller then persists the
// delta); on any non-promotion path rec is left untouched. A hard error
// (attestor/broker failure, an invalid promoted record) is returned distinct
// from a fail-safe non-promotion, which returns (Outcome{Promoted:false}, nil).
func (p *Promoter) Promote(ctx context.Context, rec *record.Record) (Outcome, error) {
	// Root-cause pre-gate (#0094): a record without a specific root cause is
	// advice, not a trap/fix — held cheaply before ANY attestation or panel call.
	if !HasSubstantiveRootCause(rec) {
		return skip("no substantive root_cause — held (advice, not a trap/fix) (#0094)")
	}

	// Advisory-class panel path (ADR-0016) — before the proof path.
	if ok, _ := EligibleAdvisory(rec); ok {
		return p.promoteAdvisory(ctx, rec)
	}

	// Prose-class panel path (ADR-0020) — a no-repro, no-source lesson, judged by the
	// cross-family panel. Before the proof eligibility check, which would otherwise skip
	// it as "no executable proof — left for a human".
	if ok, _ := EligibleProse(rec); ok {
		return p.promoteProse(ctx, rec)
	}

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

func (p *Promoter) promoteAdvisory(ctx context.Context, rec *record.Record) (Outcome, error) {
	if p.advisoryPanel == nil {
		return skip("no advisory panel configured — left for a human")
	}
	// Born-stale gate (#0071, companion to #302): an advisory whose runtime is
	// already end-of-life would be flagged stale by the D2 guard the instant it
	// became validated — promoting it just manufactures a validated record the next
	// staleness run demotes, and reds the guard test that gates the validate PR.
	// Refuse it here, BEFORE the costly panel; it stays quarantined.
	if p.stalenessGate != nil {
		if f := p.stalenessGate(ctx, rec); f != nil {
			return skip("runtime is end-of-life — born-stale, not promoted (#0071): " + f.Issue)
		}
	}
	// Consistency pre-gate (#0061): a deterministic, LLM-free hold for the precise
	// transcription-defect classes (null-fixed/fix-text contradiction, malformed
	// package path), computed LIVE from the record. This protects the LEGACY
	// backlog — records ingested before the ingest gate carry no STORED
	// consistency_flags, so the validate rule never fires for them; without this the
	// panel could approve an exp-0061-class contradiction. Fail-safe: held, never
	// promoted. source-url-id-mismatch is advisory-only and is NOT gated here.
	if defects := record.AdvisoryBlockingDefects(rec); len(defects) > 0 {
		return skip(fmt.Sprintf("consistency defect — held, not promoted (fail-safe, #0061): %v", defects))
	}
	verdict, err := p.advisoryPanel.Judge(ctx, judge.Request{Record: rec})
	if err != nil {
		return Outcome{Reason: "advisory panel unavailable — stays quarantined (fail-safe): " + err.Error(), Verdict: verdict, Unjudged: true}, nil
	}
	if !verdict.Approved() {
		return Outcome{Reason: "advisory panel did not approve — stays quarantined", Verdict: verdict}, nil
	}

	panel := panelVerdicts(p.advisoryPanel)
	origStatus, origValidatedAt, origPromotion := rec.Status, rec.Provenance.ValidatedAt, rec.Provenance.Promotion
	validatedAt := p.now()
	rec.Status = "validated"
	rec.Provenance.ValidatedAt = &validatedAt
	rec.Provenance.Promotion = &record.Promotion{
		Panel:         panel,
		JudgeModel:    verdict.Model,
		JudgeDecision: string(verdict.Decision),
	}
	if err := record.Validate(rec); err != nil {
		rec.Status, rec.Provenance.ValidatedAt, rec.Provenance.Promotion = origStatus, origValidatedAt, origPromotion
		return Outcome{}, fmt.Errorf("promote %s: promoted record is invalid (not persisted): %w", rec.ID, err)
	}
	return Outcome{Promoted: true, Verdict: verdict}, nil
}

// promoteProse promotes a prose-class record (ADR-0020) via the cross-family panel —
// no attestation, no cited source: the panel judges the advice on its own coherence +
// safety (poison gating, ProsePanelSystemV2). Fail-safe in every direction, exactly like
// the advisory path: a nil panel, any member error/timeout, or any dissent leaves the
// record quarantined. The content-screen is already enforced by EligibleProse.
func (p *Promoter) promoteProse(ctx context.Context, rec *record.Record) (Outcome, error) {
	if p.prosePanel == nil {
		return skip("no prose panel configured — left for a human (ADR-0013 §5)")
	}
	// Born-stale gate (ADR-0016 §7, #0071): a lesson whose valid.until is already past is
	// not promote-worthy; held, quarantined, before the costly panel.
	if p.stalenessGate != nil {
		if f := p.stalenessGate(ctx, rec); f != nil {
			return skip("record is born-stale, not promoted (#0071): " + f.Issue)
		}
	}
	verdict, err := p.prosePanel.Judge(ctx, judge.Request{Record: rec, Prose: true})
	if err != nil {
		return Outcome{Reason: "prose panel unavailable — stays quarantined (fail-safe): " + err.Error(), Verdict: verdict, Unjudged: true}, nil
	}
	if !verdict.Approved() {
		return Outcome{Reason: "prose panel did not approve — stays quarantined", Verdict: verdict}, nil
	}

	panel := panelVerdicts(p.prosePanel)
	origStatus, origValidatedAt, origPromotion := rec.Status, rec.Provenance.ValidatedAt, rec.Provenance.Promotion
	validatedAt := p.now()
	rec.Status = "validated"
	rec.Provenance.ValidatedAt = &validatedAt
	rec.Provenance.Promotion = &record.Promotion{
		Panel:         panel,
		JudgeModel:    verdict.Model,
		JudgeDecision: string(verdict.Decision),
	}
	if err := record.Validate(rec); err != nil {
		rec.Status, rec.Provenance.ValidatedAt, rec.Provenance.Promotion = origStatus, origValidatedAt, origPromotion
		return Outcome{}, fmt.Errorf("promote %s: promoted record is invalid (not persisted): %w", rec.ID, err)
	}
	return Outcome{Promoted: true, Verdict: verdict}, nil
}

type panelMemberRecorder interface {
	PanelMembers() []judge.Verdict
}

func panelVerdicts(j judge.Judge) []record.PanelVerdict {
	pr, ok := j.(panelMemberRecorder)
	if !ok {
		return nil
	}
	members := pr.PanelMembers()
	out := make([]record.PanelVerdict, 0, len(members))
	for _, mv := range members {
		out = append(out, record.PanelVerdict{
			JudgeModel:    mv.Model,
			JudgeDecision: string(mv.Decision),
		})
	}
	return out
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
