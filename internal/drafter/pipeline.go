// SPDX-License-Identifier: AGPL-3.0-only

package drafter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// Outcome is what the pipeline decided for one record.
type Outcome struct {
	RecordID    string            // the record processed
	Drafted     bool              // a candidate repro was produced (false ⇒ unsupported)
	Attached    bool              // it held under the gate and is now in guard.repros
	ReproPath   string            // corpus-relative path of the drafted repro
	Attestation repro.Attestation // the gate's evidence (zero when not gated)
	Reason      string            // why not attached (skip/reject), for the log
}

// Pipeline composes the three steps of ADR-0011 §8: Drafter → broker Gate →
// attach-or-reject. A drafter writes a candidate repro under root, the gate
// PROVES it (fail-pre / pass-post, offline), and a holding attestation attaches
// the repro into the record's guard — still quarantined; promotion stays the
// human PR step (#0020). A rejected draft is dropped and its files removed.
//
// Drafters are tried in order; the first that does not return ErrUnsupported
// produces the candidate (deterministic templates first, the model drafter as a
// fallback for what they can't cover). The winning drafter's Name is recorded on
// the attached repro's label, so a model-origin repro is auditably distinct.
//
// The revalidator MUST be constructed with the same corpus root, so its gate
// resolves the path the drafter just wrote.
type Pipeline struct {
	drafters []Drafter
	rv       *repro.Revalidator
	root     string
}

// NewPipeline wires an ordered set of drafters and a revalidator over a shared
// corpus root. At least one drafter should be supplied; with none, every record
// is reported unsupported.
func NewPipeline(rv *repro.Revalidator, root string, drafters ...Drafter) *Pipeline {
	return &Pipeline{drafters: drafters, rv: rv, root: root}
}

// Run drafts a repro for rec, gates it, and attaches it on a holding attestation.
// It returns a non-nil error only for an unexpected failure (a hard draft error
// or a gate crash); an unsupported record or a clean gate rejection are normal
// Outcomes with Attached=false. On attach, rec.Guard.Repros gains the proven
// repro; otherwise rec is left exactly as it came in.
func (p *Pipeline) Run(ctx context.Context, rec *record.Record) (Outcome, error) {
	out := Outcome{RecordID: rec.ID}

	// Try drafters in order: the first that does not decline (ErrUnsupported) owns
	// the candidate. A non-ErrUnsupported error is a hard failure and surfaces.
	var dir, drafterName string
	for _, d := range p.drafters {
		rp, err := d.Draft(ctx, p.root, rec)
		if errors.Is(err, ErrUnsupported) {
			continue
		}
		if err != nil {
			return out, fmt.Errorf("draft %s: %w", rec.ID, err)
		}
		if rp == "" {
			continue // defensive: a drafter that returns no path has declined
		}
		dir, drafterName = rp, d.Name()
		break
	}
	if drafterName == "" {
		out.Reason = "unsupported by all drafters"
		return out, nil
	}
	out.Drafted, out.ReproPath = true, dir

	// Tentatively attach so the gate resolves it, then keep or drop on the verdict.
	if rec.Guard == nil {
		rec.Guard = &record.Guard{}
	}
	rec.Guard.Repros = append(rec.Guard.Repros, record.Repro{
		Path:  dir,
		Kind:  "positive",
		Label: "auto-generated " + drafterName + " repro",
	})

	_, atts, err := p.rv.RunWithAttestations(ctx, []*record.Record{rec})
	if err != nil {
		p.detach(rec, dir)
		return out, fmt.Errorf("gate %s: %w", rec.ID, err)
	}
	if len(atts) == 0 { // defensive: a record with a repro always attests
		p.detach(rec, dir)
		out.Reason = "gate produced no attestation"
		return out, nil
	}
	out.Attestation = atts[0]
	if atts[0].Holds && !atts[0].Inconclusive {
		out.Attached = true
		return out, nil
	}

	// Auto-reject: the draft did not truly fail-pre / pass-post. Drop it and
	// remove the orphan files so the corpus isn't polluted by failed drafts.
	p.detach(rec, dir)
	out.Reason = "gate rejected (repro did not hold)"
	return out, nil
}

// detach removes the tentatively-attached repro from the guard and deletes its
// staged files.
func (p *Pipeline) detach(rec *record.Record, dir string) {
	if rec.Guard != nil && len(rec.Guard.Repros) > 0 {
		last := rec.Guard.Repros[len(rec.Guard.Repros)-1]
		if last.Path == dir {
			rec.Guard.Repros = rec.Guard.Repros[:len(rec.Guard.Repros)-1]
		}
	}
	_ = os.RemoveAll(filepath.Join(p.root, filepath.FromSlash(dir)))
}
