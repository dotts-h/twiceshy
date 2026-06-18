// SPDX-License-Identifier: AGPL-3.0-only

package repro

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/doctor"
	"github.com/dotts-h/twiceshy/internal/record"
)

// Revalidator is the execution-validation harness proper (ADR-0011 §3, #0020):
// the doctor that PROVES a quarantined record by running its repro test-set in
// the sandbox broker across a version matrix. It implements doctor.Doctor and is
// **report-only** — it emits a Finding + a structured Attestation proposing
// promotion, but never mutates a record. A human flips `validated`/`validated_at`
// in the PR (the git/PR trust boundary, ADR-0001/0008). Go ecosystem first.
//
// Exit-code convention of a repro (docs/SCHEMA.md): 0 = the claim it encodes
// holds (a positive reproduces fail→pass; a negative dead-end still fails as
// expected), 75 = the environment can't run it (skip), any other code = the
// claim no longer holds (the world drifted → propose `stale`).
type Revalidator struct {
	broker Broker
	root   string // corpus root, to read repro scripts by their record-relative path
	matrix []MatrixEntry
	now    func() time.Time
}

// MatrixEntry is one image the test-set is run against. A matrix of several Go
// toolchain images lets the revalidator observe the version range over which a
// record's claim holds (and so bound `applies_to` empirically). Images must be
// digest-pinned and in the broker's allowlist.
type MatrixEntry struct {
	Label string // human label, e.g. "go1.25"
	Image string // digest-pinned image ref
}

// DefaultGoMatrix is the single pinned Go image from #0017. Extend it with more
// pinned toolchain images to widen empirical version-bound derivation.
var DefaultGoMatrix = []MatrixEntry{{Label: "go1.25", Image: PinnedGoImage}}

// Attestation is the structured evidence of one record's revalidation: exactly
// what ran (image digests, per-repro exit codes) and what it proved. It is the
// artifact a reviewer reads before flipping `validated` in the PR.
type Attestation struct {
	RecordID        string         `json:"record_id"`
	RanAt           string         `json:"ran_at"`
	Matrix          []MatrixResult `json:"matrix"`
	Holds           bool           `json:"holds"`            // every non-skipped repro held on every non-skipped entry
	Inconclusive    bool           `json:"inconclusive"`     // nothing actually ran (all skipped / no repros)
	ReproducedUnder []string       `json:"reproduced_under"` // matrix labels where the whole set held
	Note            string         `json:"note,omitempty"`   // applies_to sanity-check, etc.
}

// MatrixResult is one matrix entry's outcome for a record.
type MatrixResult struct {
	Label   string         `json:"label"`
	Image   string         `json:"image"`
	Repros  []ReproOutcome `json:"repros"`
	Skipped bool           `json:"skipped"` // every repro skipped (env couldn't run them)
}

// ReproOutcome is one repro's result on one matrix entry.
type ReproOutcome struct {
	Path     string `json:"path"`
	Kind     string `json:"kind"` // positive | negative
	ExitCode int    `json:"exit_code"`
	Status   string `json:"status"` // holds | broken | skipped | error
	Detail   string `json:"detail,omitempty"`
}

// RevalOption configures a Revalidator.
type RevalOption func(*Revalidator)

// WithMatrix sets the version matrix (default DefaultGoMatrix).
func WithMatrix(m []MatrixEntry) RevalOption { return func(r *Revalidator) { r.matrix = m } }

// WithClock injects the clock (tests pin it).
func WithClock(now func() time.Time) RevalOption { return func(r *Revalidator) { r.now = now } }

// NewRevalidator returns the revalidate doctor. broker runs the repros; root is
// the corpus root used to resolve each record's repro-script path.
func NewRevalidator(broker Broker, root string, opts ...RevalOption) *Revalidator {
	r := &Revalidator{
		broker: broker,
		root:   root,
		matrix: DefaultGoMatrix,
		now:    func() time.Time { return time.Now().UTC() },
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Name implements doctor.Doctor.
func (*Revalidator) Name() string { return "revalidate" }

// Run implements doctor.Doctor: revalidate the records and return proposed
// promotions/demotions. Attestations are discarded here; callers that want the
// full evidence use RunWithAttestations.
func (r *Revalidator) Run(ctx context.Context, recs []*record.Record) (doctor.Report, error) {
	rep, _, err := r.RunWithAttestations(ctx, recs)
	return rep, err
}

// RunWithAttestations revalidates each record that carries a repro test-set and
// returns both the report and the structured attestations. It never mutates a
// record.
func (r *Revalidator) RunWithAttestations(ctx context.Context, recs []*record.Record) (doctor.Report, []Attestation, error) {
	rep := doctor.Report{Doctor: r.Name()}
	var atts []Attestation
	for _, rec := range recs {
		repros := reprosOf(rec)
		if len(repros) == 0 {
			continue // nothing executable to revalidate
		}
		att, finding := r.revalidateOne(ctx, rec, repros)
		atts = append(atts, att)
		rep.Findings = append(rep.Findings, finding)
	}
	return rep, atts, nil
}

// reproRef is one repro to run (path + kind), normalizing the legacy single
// guard.repro into a positive.
type reproRef struct {
	path string
	kind string
}

func reprosOf(rec *record.Record) []reproRef {
	if rec.Guard == nil {
		return nil
	}
	var out []reproRef
	if rec.Guard.Repro != nil && *rec.Guard.Repro != "" {
		out = append(out, reproRef{path: *rec.Guard.Repro, kind: "positive"})
	}
	for _, rp := range rec.Guard.Repros {
		out = append(out, reproRef{path: rp.Path, kind: rp.Kind})
	}
	return out
}

// revalidateOne runs a record's whole test-set across the matrix and turns the
// results into an Attestation + a proposed Finding.
func (r *Revalidator) revalidateOne(ctx context.Context, rec *record.Record, repros []reproRef) (Attestation, doctor.Finding) {
	att := Attestation{RecordID: rec.ID, RanAt: r.now().Format(time.RFC3339)}
	holds := true
	ranAnything := false

	for _, entry := range r.matrix {
		mr := MatrixResult{Label: entry.Label, Image: entry.Image}
		entrySkipped := true
		for _, rp := range repros {
			out := r.runRepro(ctx, entry, rp)
			mr.Repros = append(mr.Repros, out)
			switch out.Status {
			case "holds":
				entrySkipped = false
				ranAnything = true
			case "skipped":
				// does not count for or against
			default: // broken | error
				entrySkipped = false
				ranAnything = true
				holds = false
			}
		}
		mr.Skipped = entrySkipped
		att.Matrix = append(att.Matrix, mr)
		if !entrySkipped && entryHolds(mr) {
			att.ReproducedUnder = append(att.ReproducedUnder, entry.Label)
		}
	}

	att.Holds = holds && ranAnything
	att.Inconclusive = !ranAnything
	att.Note = appliesToSanityCheck(rec, att.ReproducedUnder)

	return att, r.toFinding(rec, att)
}

// runRepro runs a single repro on a single matrix entry through the broker.
func (r *Revalidator) runRepro(ctx context.Context, entry MatrixEntry, rp reproRef) ReproOutcome {
	out := ReproOutcome{Path: rp.path, Kind: rp.kind}
	body, err := r.readRepro(rp.path)
	if err != nil {
		out.Status, out.Detail = "error", err.Error()
		return out
	}
	base := path.Base(filepath.ToSlash(rp.path))
	res, err := r.broker.Run(ctx, Job{
		Image:   entry.Image,
		Files:   map[string][]byte{base: body},
		Execute: []string{"sh", workDir + "/" + base},
		Env:     map[string]string{"TWICESHY_MATRIX_LABEL": entry.Label},
	})
	if err != nil {
		out.Status, out.Detail = "error", err.Error()
		return out
	}
	out.ExitCode = res.Execute.ExitCode
	switch res.Execute.ExitCode {
	case 0:
		out.Status = "holds"
	case 75:
		out.Status = "skipped"
		out.Detail = "environment cannot run repro (EX_TEMPFAIL)"
	default:
		out.Status = "broken"
		out.Detail = strings.TrimSpace(firstLine(res.Execute.Stdout, res.Execute.Stderr))
	}
	if res.Execute.TimedOut {
		out.Status, out.Detail = "broken", "timed out"
	}
	return out
}

// readRepro reads a repro script confined to under the corpus root.
func (r *Revalidator) readRepro(p string) ([]byte, error) {
	clean := filepath.Clean(filepath.FromSlash(p))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("repro path %q escapes the corpus root", p)
	}
	return os.ReadFile(filepath.Join(r.root, clean))
}

// toFinding turns an attestation into the proposed delta for human review.
func (r *Revalidator) toFinding(rec *record.Record, att Attestation) doctor.Finding {
	f := doctor.Finding{RecordID: rec.ID, Path: rec.Path}
	switch {
	case att.Inconclusive:
		f.Issue = "revalidation inconclusive — every repro skipped (environment could not run them)"
		f.Proposal = "no change; re-run where the repro's toolchain/deps are available"
	case att.Holds && rec.Status == "quarantined":
		f.Issue = fmt.Sprintf("repro test-set reproduces fail→pass under %s", strings.Join(att.ReproducedUnder, ", "))
		f.Proposal = "promote to validated: set status=validated and provenance.validated_at (human, in the PR)"
	case att.Holds:
		f.Issue = fmt.Sprintf("repro test-set still holds under %s", strings.Join(att.ReproducedUnder, ", "))
		f.Proposal = "no change; record remains valid (attestation refreshed)"
	default:
		broken := brokenRepros(att)
		f.Issue = fmt.Sprintf("repro no longer holds (%s) — the world may have drifted", strings.Join(broken, "; "))
		f.Proposal = "propose stale: mark status=stale for review (supersede, never delete)"
	}
	if att.Note != "" {
		f.Issue += " [" + att.Note + "]"
	}
	return f
}

// entryHolds reports whether every non-skipped repro on an entry held.
func entryHolds(mr MatrixResult) bool {
	any := false
	for _, o := range mr.Repros {
		switch o.Status {
		case "holds":
			any = true
		case "skipped":
		default:
			return false
		}
	}
	return any
}

func brokenRepros(att Attestation) []string {
	seen := map[string]bool{}
	var out []string
	for _, mr := range att.Matrix {
		for _, o := range mr.Repros {
			if o.Status == "broken" || o.Status == "error" {
				k := o.Path + " on " + mr.Label
				if !seen[k] {
					seen[k] = true
					out = append(out, fmt.Sprintf("%s on %s: exit %d", path.Base(o.Path), mr.Label, o.ExitCode))
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// appliesToSanityCheck compares the empirically-reproduced matrix labels against
// the record's declared Go runtime range, surfacing agreement or conflict. It is
// advisory (report-only); the human sets the final bounds.
func appliesToSanityCheck(rec *record.Record, reproducedUnder []string) string {
	if len(reproducedUnder) == 0 {
		return ""
	}
	for _, a := range rec.AppliesTo {
		if v, ok := a.Runtime["go"]; ok && v != "" {
			return fmt.Sprintf("declared go runtime %q; reproduced under %s — verify applies_to bounds",
				v, strings.Join(reproducedUnder, ", "))
		}
	}
	return "no declared go runtime bound; empirical: reproduced under " + strings.Join(reproducedUnder, ", ")
}

func firstLine(stdout, stderr string) string {
	s := stderr
	if strings.TrimSpace(s) == "" {
		s = stdout
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
