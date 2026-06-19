// SPDX-License-Identifier: AGPL-3.0-only

// Package record implements parsing and validation of twiceshy experience
// records — the normative spec is docs/SCHEMA.md, mirrored by
// schema/experience-record.v1.schema.json. Records are YAML frontmatter
// plus a non-empty markdown narrative body.
package record

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the record schema this parser understands.
const SchemaVersion = 1

// ValidID reports whether id is a well-formed record id (exp-NNNN, ≥4 digits) —
// the shared predicate behind `id`, `superseded_by`, and `disputes`.
func ValidID(id string) bool { return reID.MatchString(id) }

// Kinds and statuses, per docs/SCHEMA.md.
var (
	Kinds    = []string{"trap", "fix", "dead-end", "convention", "workflow"}
	Statuses = []string{"quarantined", "validated", "stale", "superseded", "disputed"}
)

// Record is a parsed experience record: frontmatter fields plus the
// narrative body and the corpus-relative path it was loaded from.
type Record struct {
	SchemaVersion int         `yaml:"schema_version"`
	ID            string      `yaml:"id"`
	Kind          string      `yaml:"kind"`
	Status        string      `yaml:"status"`
	Title         string      `yaml:"title"`
	Symptom       *Symptom    `yaml:"symptom,omitempty"`
	AppliesTo     []AppliesTo `yaml:"applies_to,omitempty"`
	Resolution    *Resolution `yaml:"resolution,omitempty"`
	Guard         *Guard      `yaml:"guard,omitempty"`
	Provenance    Provenance  `yaml:"provenance"`

	// Body is the markdown narrative below the frontmatter; Raw is the
	// complete source file (what get_experience serves); Path is the
	// corpus-relative file path.
	Body string `yaml:"-"`
	Raw  []byte `yaml:"-"`
	Path string `yaml:"-"`
}

type Symptom struct {
	Summary         string        `yaml:"summary"`
	ErrorSignatures []string      `yaml:"error_signatures,omitempty"`
	Fingerprints    *Fingerprints `yaml:"fingerprints,omitempty"`
}

// Fingerprints are *additive*, externally sourced fingerprints (e.g. from
// Sentry); the indexer always derives its own from ErrorSignatures.
type Fingerprints struct {
	App     []string `yaml:"app"`
	Generic []string `yaml:"generic"`
}

type AppliesTo struct {
	Ecosystem string            `yaml:"ecosystem,omitempty"`
	Package   string            `yaml:"package,omitempty"`
	Versions  *VersionRange     `yaml:"versions,omitempty"`
	Runtime   map[string]string `yaml:"runtime,omitempty"`
}

type VersionRange struct {
	Introduced *string `yaml:"introduced"`
	Fixed      *string `yaml:"fixed"`
}

type Resolution struct {
	RootCause string    `yaml:"root_cause,omitempty"`
	Fix       string    `yaml:"fix,omitempty"`
	DeadEnds  []DeadEnd `yaml:"dead_ends,omitempty"`
}

type DeadEnd struct {
	Tried       string `yaml:"tried"`
	WhyItFailed string `yaml:"why_it_failed"`
}

// ReproKinds are the allowed guard.repros[].kind values.
var ReproKinds = []string{"positive", "negative"}

// Repro is one executable proof in a record's guard test-set.
// Kind "positive" = fail-to-pass (fails pre-fix, passes post-fix);
// "negative" = a dead-end that must stay failing (proves "don't try Z").
type Repro struct {
	Path  string `yaml:"path"`
	Kind  string `yaml:"kind"`
	Label string `yaml:"label,omitempty"`
}

type Guard struct {
	Repro        *string `yaml:"repro"`
	Repros       []Repro `yaml:"repros,omitempty"`
	GuardingTest *string `yaml:"guarding_test"`
}

type Provenance struct {
	Source      Source   `yaml:"source"`
	RecordedAt  string   `yaml:"recorded_at"`
	ValidatedAt *string  `yaml:"validated_at"`
	Valid       Validity `yaml:"valid"`
	// SourceLicense and SourceURL are additive, optional importer-provenance
	// fields (ADR-0003 §4): they let the pack builder mechanically keep
	// commercial packs license-clean. SourceLicense is an SPDX id or the
	// SourceLicenseFactsOnly sentinel; both are omitted when empty.
	SourceLicense string  `yaml:"source_license,omitempty"`
	SourceURL     string  `yaml:"source_url,omitempty"`
	SupersededBy  *string `yaml:"superseded_by"`
	// Disputes is the additive, optional link an outcome-report counter-record
	// (#0031) carries to the existing record it contests — an exp-id, like
	// SupersededBy. #0032 follows it to re-run the original repro plus the
	// counter; it is evidence, not a verdict, so it never mutates the target.
	Disputes *string `yaml:"disputes,omitempty"`
	// Promotion is the additive, optional audit trail an autonomous promotion
	// (#0029, ADR-0013) stamps in: the holding attestation it rode and the
	// diverse judge's verdict. Set only by the promoter; nil on a human-validated
	// or quarantined record.
	Promotion *Promotion `yaml:"promotion,omitempty"`
	// Demotion is the symmetric audit trail an autonomous demotion (#0032,
	// ADR-0013 §3) stamps in when reproduced counter-evidence + a judge PASS
	// demoted this record to stale: the counter-attestation, the verdict, and the
	// report that triggered it. Set only by the counter-evidence gate.
	Demotion *Demotion `yaml:"demotion,omitempty"`
	Usage    *Usage    `yaml:"usage,omitempty"`
	// SecurityFlags records hazards the ingestion safety gate detected
	// (#0011), e.g. "secret:aws-access-key". A flagged record is documented and
	// quarantined but MUST NOT be promoted to validated (see validateProvenance).
	SecurityFlags []string `yaml:"security_flags,omitempty"`
}

// SourceLicenseFactsOnly is the source_license sentinel for a record that
// distills only non-copyrightable facts (no third-party expression), so it
// carries no license obligation. (ADR-0003 §4)
const SourceLicenseFactsOnly = "none (facts only)"

type Source struct {
	Author  string  `yaml:"author"`
	Session *string `yaml:"session"`
	PR      *string `yaml:"pr"`
}

type Validity struct {
	From  string  `yaml:"from"`
	Until *string `yaml:"until"`
}

type Usage struct {
	Retrieved        int     `yaml:"retrieved"`
	ConfirmedHelpful int     `yaml:"confirmed_helpful"`
	LastHit          *string `yaml:"last_hit"`
}

// Promotion records why an autonomous quarantined→validated flip was allowed
// (#0029, ADR-0013 §1–2): the holding broker attestation and the diverse
// judge's verdict, carried in the git commit itself as the audit trail.
type Promotion struct {
	AttestedAt      string   `yaml:"attested_at"`                // the attestation's ran_at (RFC3339)
	ReproducedUnder []string `yaml:"reproduced_under,omitempty"` // matrix labels the test-set held under
	JudgeModel      string   `yaml:"judge_model"`                // the diverse model that approved
	JudgeDecision   string   `yaml:"judge_decision"`             // the verdict decision (an approval)
}

// Demotion records why an autonomous validated→stale flip was allowed (#0032,
// ADR-0013 §3): the counter-attestation that reproduced the failure, the diverse
// judge's verdict, and the outcome-report that triggered it.
type Demotion struct {
	AttestedAt    string `yaml:"attested_at"`    // the counter-attestation's ran_at (RFC3339)
	JudgeModel    string `yaml:"judge_model"`    // the diverse model that approved the demotion
	JudgeDecision string `yaml:"judge_decision"` // the verdict decision
	Report        string `yaml:"report"`         // the outcome-report (exp-NNNN) that triggered it
}

var (
	reID          = regexp.MustCompile(`^exp-[0-9]{4,}$`)
	reDate        = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
	reFingerprint = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	// experience/<YYYY>/<NNNN...>-<slug>.md
	reRecordPath = regexp.MustCompile(`^experience/([0-9]{4})/([0-9]{4,})-[a-z0-9-]+\.md$`)
	// A single SPDX license id (e.g. MIT, Apache-2.0, CC-BY-4.0); not a full
	// SPDX expression (no AND/OR/WITH) — one source, one license id.
	reSPDX = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]*$`)
	// source_url must be an http(s) URL with a non-empty host/path.
	reHTTPURL = regexp.MustCompile(`^https?://[^\s]+$`)
)

// Parse parses and validates one record. path is the corpus-relative file
// path (filename and year-directory rules are part of validation).
func Parse(path string, src []byte) (*Record, error) {
	front, body, err := splitFrontmatter(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	var rec Record
	dec := yaml.NewDecoder(bytes.NewReader(front))
	dec.KnownFields(true)
	if err := dec.Decode(&rec); err != nil {
		return nil, fmt.Errorf("%s: frontmatter: %w", path, err)
	}
	rec.Body = strings.TrimSpace(body)
	rec.Raw = src
	rec.Path = filepath.ToSlash(path)

	if err := rec.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &rec, nil
}

// ParseFile parses root-relative rel under the corpus root.
func ParseFile(root, rel string) (*Record, error) {
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	return Parse(rel, src)
}

// LoadCorpus loads every record under root/experience, applies corpus-level
// checks (unique ids, resolvable superseded_by, existing repro files), and
// returns the records sorted by id.
func LoadCorpus(root string) ([]*Record, error) {
	expDir := filepath.Join(root, "experience")
	var recs []*Record
	err := filepath.WalkDir(expDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !reRecordPath.MatchString(rel) {
			return nil // repro scripts, READMEs, scratch files
		}
		rec, err := ParseFile(root, rel)
		if err != nil {
			return err
		}
		recs = append(recs, rec)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })

	byID := make(map[string]*Record, len(recs))
	var errs []error
	for _, r := range recs {
		if dup, ok := byID[r.ID]; ok {
			errs = append(errs, fmt.Errorf("duplicate id %s in %s and %s", r.ID, dup.Path, r.Path))
		}
		byID[r.ID] = r
	}
	for _, r := range recs {
		if sb := r.Provenance.SupersededBy; sb != nil {
			if _, ok := byID[*sb]; !ok {
				errs = append(errs, fmt.Errorf("%s: superseded_by %s does not exist in the corpus", r.Path, *sb))
			}
		}
		if r.Guard != nil {
			if r.Guard.Repro != nil {
				repro := filepath.Join(root, filepath.FromSlash(*r.Guard.Repro))
				if _, err := os.Stat(repro); err != nil {
					errs = append(errs, fmt.Errorf("%s: guard.repro %s: %w", r.Path, *r.Guard.Repro, err))
				}
			}
			for _, repro := range r.Guard.Repros {
				p := filepath.Join(root, filepath.FromSlash(repro.Path))
				if _, err := os.Stat(p); err != nil {
					errs = append(errs, fmt.Errorf("%s: guard.repros path %s: %w", r.Path, repro.Path, err))
				}
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return recs, nil
}

func splitFrontmatter(src []byte) (front []byte, body string, err error) {
	const fence = "---\n"
	if !bytes.HasPrefix(src, []byte(fence)) {
		return nil, "", errors.New("missing leading frontmatter fence")
	}
	rest := src[len(fence):]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return nil, "", errors.New("unterminated frontmatter fence")
	}
	front = rest[:end+1]
	body = string(rest[end+len("\n---\n"):])
	if strings.TrimSpace(body) == "" {
		return nil, "", errors.New("narrative body must be non-empty")
	}
	return front, body, nil
}

// Validate runs the full record validation on a programmatically-constructed
// Record — the same checks Parse applies after decoding frontmatter. The write
// path (ingest) builds a Record in memory and validates it before persisting.
// Path, Body, and the frontmatter fields must already be set.
func Validate(r *Record) error { return r.validate() }

func (r *Record) validate() error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	if r.SchemaVersion != SchemaVersion {
		fail("schema_version %d is not supported (want %d)", r.SchemaVersion, SchemaVersion)
	}
	if !reID.MatchString(r.ID) {
		fail("id %q does not match %s", r.ID, reID)
	}
	if !contains(Kinds, r.Kind) {
		fail("kind %q is not one of %v", r.Kind, Kinds)
	}
	if !contains(Statuses, r.Status) {
		fail("status %q is not one of %v", r.Status, Statuses)
	}
	if n := len([]rune(r.Title)); n < 8 || n > 120 {
		fail("title length %d is outside 8..120", n)
	}
	if strings.TrimSpace(r.Body) == "" {
		fail("narrative body must be non-empty")
	}

	r.validatePath(fail)
	r.validateSymptom(fail)
	r.validateAppliesTo(fail)
	r.validateResolution(fail)
	r.validateGuard(fail)
	r.validateProvenance(fail)

	return errors.Join(errs...)
}

func (r *Record) validatePath(fail func(string, ...any)) {
	m := reRecordPath.FindStringSubmatch(r.Path)
	if m == nil {
		fail("path %q does not match experience/YYYY/NNNN-slug.md", r.Path)
		return
	}
	if id := "exp-" + m[2]; id != r.ID {
		fail("filename number %s does not match id %s", m[2], r.ID)
	}
	if year := r.Provenance.RecordedAt; len(year) >= 4 && m[1] != year[:4] {
		fail("year directory %s does not match recorded_at %s", m[1], year)
	}
}

func (r *Record) validateSymptom(fail func(string, ...any)) {
	episodic := r.Kind == "trap" || r.Kind == "fix" || r.Kind == "dead-end"
	if r.Symptom == nil {
		if episodic {
			fail("kind %s requires a symptom block", r.Kind)
		}
		return
	}
	if strings.TrimSpace(r.Symptom.Summary) == "" {
		fail("symptom.summary must be non-empty")
	}
	for _, sig := range r.Symptom.ErrorSignatures {
		if strings.TrimSpace(sig) == "" {
			fail("error_signatures must not contain empty entries")
		}
	}
	if fp := r.Symptom.Fingerprints; fp != nil {
		for _, f := range append(append([]string{}, fp.App...), fp.Generic...) {
			if !reFingerprint.MatchString(f) {
				fail("explicit fingerprint %q does not match %s", f, reFingerprint)
			}
		}
	}
}

func (r *Record) validateAppliesTo(fail func(string, ...any)) {
	for i, a := range r.AppliesTo {
		if a.Ecosystem == "" && a.Package == "" && len(a.Runtime) == 0 {
			fail("applies_to[%d] needs at least one of ecosystem, package, runtime", i)
		}
	}
}

func (r *Record) validateResolution(fail func(string, ...any)) {
	episodic := r.Kind == "trap" || r.Kind == "fix" || r.Kind == "dead-end"
	if r.Resolution == nil {
		if episodic {
			fail("kind %s requires a resolution block", r.Kind)
		}
		return
	}
	if r.Kind == "trap" || r.Kind == "fix" {
		if strings.TrimSpace(r.Resolution.RootCause) == "" {
			fail("kind %s requires resolution.root_cause", r.Kind)
		}
		if strings.TrimSpace(r.Resolution.Fix) == "" {
			fail("kind %s requires resolution.fix", r.Kind)
		}
	}
	if r.Kind == "dead-end" && len(r.Resolution.DeadEnds) == 0 {
		fail("kind dead-end requires resolution.dead_ends")
	}
	for i, d := range r.Resolution.DeadEnds {
		if strings.TrimSpace(d.Tried) == "" || strings.TrimSpace(d.WhyItFailed) == "" {
			fail("dead_ends[%d] requires both tried and why_it_failed", i)
		}
	}
}

func (r *Record) validateGuard(fail func(string, ...any)) {
	if r.Guard != nil {
		seen := make(map[string]struct{}, len(r.Guard.Repros))
		for i, repro := range r.Guard.Repros {
			if strings.TrimSpace(repro.Path) == "" {
				fail("guard.repros[%d].path must be non-empty", i)
			}
			if !contains(ReproKinds, repro.Kind) {
				fail("guard.repros[%d].kind %q is not one of %v", i, repro.Kind, ReproKinds)
			}
			if repro.Path != "" {
				if _, dup := seen[repro.Path]; dup {
					fail("guard.repros contains duplicate path %q", repro.Path)
				}
				seen[repro.Path] = struct{}{}
			}
		}
	}
	needsGuard := r.Status == "validated" && (r.Kind == "trap" || r.Kind == "fix")
	if !needsGuard {
		return
	}
	// A validated trap/fix needs executable proof: either a named guarding test
	// (the unit test that keeps the fix fixed) OR a positive repro (the
	// fail-to-pass script the execution-validation harness ran, ADR-0011). The
	// repro IS the proof for execution-validated records, so it satisfies this on
	// its own — requiring an extra Go unit test would defeat the harness.
	if r.Guard != nil && r.Guard.GuardingTest != nil && strings.TrimSpace(*r.Guard.GuardingTest) != "" {
		return
	}
	if r.Guard != nil && r.hasPositiveRepro() {
		return
	}
	fail("validated %s requires guard.guarding_test or a positive guard repro", r.Kind)
}

// HasPositiveRepro reports whether r carries an executable positive
// (fail-to-pass) proof — the legacy single guard.repro or a guard.repros entry
// of kind "positive". A record with only a negative (dead-end) repro is NOT
// proven. Exposed so corpus tooling (the drafter pipeline) applies the same
// positive-proof rule the validator does, instead of re-deriving it.
func HasPositiveRepro(r *Record) bool { return r.hasPositiveRepro() }

// hasPositiveRepro reports whether the guard carries at least one positive
// fail-to-pass repro (the legacy single guard.repro is a positive).
func (r *Record) hasPositiveRepro() bool {
	if r.Guard == nil {
		return false
	}
	if r.Guard.Repro != nil && strings.TrimSpace(*r.Guard.Repro) != "" {
		return true
	}
	for _, rp := range r.Guard.Repros {
		if rp.Kind == "positive" && strings.TrimSpace(rp.Path) != "" {
			return true
		}
	}
	return false
}

func (r *Record) validateProvenance(fail func(string, ...any)) {
	p := &r.Provenance
	if strings.TrimSpace(p.Source.Author) == "" {
		fail("provenance.source.author is required")
	}
	recorded := checkDate(fail, "provenance.recorded_at", p.RecordedAt)
	from := checkDate(fail, "provenance.valid.from", p.Valid.From)

	var validated, until time.Time
	if p.ValidatedAt != nil {
		validated = checkDate(fail, "provenance.validated_at", *p.ValidatedAt)
	}
	if p.Valid.Until != nil {
		until = checkDate(fail, "provenance.valid.until", *p.Valid.Until)
	}

	if r.Status == "validated" && p.ValidatedAt == nil {
		fail("status validated requires provenance.validated_at")
	}
	if r.Status == "validated" && len(p.SecurityFlags) > 0 {
		fail("status validated is not allowed with provenance.security_flags %v — a flagged record cannot be promoted", p.SecurityFlags)
	}
	if r.Status == "superseded" {
		if p.SupersededBy == nil {
			fail("status superseded requires provenance.superseded_by")
		}
		if p.Valid.Until == nil {
			fail("status superseded requires provenance.valid.until")
		}
	}
	if p.SupersededBy != nil && !reID.MatchString(*p.SupersededBy) {
		fail("superseded_by %q does not match %s", *p.SupersededBy, reID)
	}
	if p.Disputes != nil && !reID.MatchString(*p.Disputes) {
		fail("disputes %q does not match %s", *p.Disputes, reID)
	}
	if pr := p.Promotion; pr != nil {
		if strings.TrimSpace(pr.AttestedAt) == "" {
			fail("provenance.promotion.attested_at is required")
		}
		if strings.TrimSpace(pr.JudgeModel) == "" {
			fail("provenance.promotion.judge_model is required")
		}
		if strings.TrimSpace(pr.JudgeDecision) == "" {
			fail("provenance.promotion.judge_decision is required")
		}
	}
	if d := p.Demotion; d != nil {
		if strings.TrimSpace(d.AttestedAt) == "" {
			fail("provenance.demotion.attested_at is required")
		}
		if strings.TrimSpace(d.JudgeModel) == "" {
			fail("provenance.demotion.judge_model is required")
		}
		if strings.TrimSpace(d.JudgeDecision) == "" {
			fail("provenance.demotion.judge_decision is required")
		}
		if !reID.MatchString(d.Report) {
			fail("provenance.demotion.report %q does not match %s", d.Report, reID)
		}
	}
	if !validated.IsZero() && !recorded.IsZero() && validated.Before(recorded) {
		fail("provenance.validated_at precedes recorded_at")
	}
	if !until.IsZero() && !from.IsZero() && until.Before(from) {
		fail("provenance.valid.until precedes valid.from")
	}
	if u := p.Usage; u != nil {
		if u.Retrieved < 0 || u.ConfirmedHelpful < 0 {
			fail("usage counters must be non-negative")
		}
		if u.LastHit != nil {
			checkDate(fail, "provenance.usage.last_hit", *u.LastHit)
		}
	}
	if lic := p.SourceLicense; lic != "" && lic != SourceLicenseFactsOnly && !reSPDX.MatchString(lic) {
		fail("provenance.source_license %q is not an SPDX id or %q", lic, SourceLicenseFactsOnly)
	}
	if u := p.SourceURL; u != "" && !reHTTPURL.MatchString(u) {
		fail("provenance.source_url %q is not an http(s) URL", u)
	}
}

func checkDate(fail func(string, ...any), field, v string) time.Time {
	if !reDate.MatchString(v) {
		fail("%s %q is not a YYYY-MM-DD date", field, v)
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		fail("%s %q: %v", field, v, err)
		return time.Time{}
	}
	return t
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
