// SPDX-License-Identifier: AGPL-3.0-only

// Package drafter turns a record's structured fact into a *candidate* repro that
// the broker gate can PROVE (ADR-0011 §8). A drafter is a seam, like the broker:
// a deterministic template drafter covers the cleanest classes (Go stdlib
// deprecations, this package); a cheap local-model drafter (#0026 slice 3) covers
// the rest. Both feed the same execution gate, which is what makes a cheap
// drafter safe — a draft that does not truly fail-pre / pass-post is auto-rejected.
package drafter

import (
	"context"
	"errors"

	"github.com/dotts-h/twiceshy/internal/record"
)

// ErrUnsupported means this drafter has no template for the record — not a
// failure, just "not my class". The pipeline skips it (a harder fact is left for
// the model drafter). Test with errors.Is.
var ErrUnsupported = errors.New("drafter: record not covered by this drafter")

// Drafter produces a candidate repro directory for a record, staged under the
// corpus root, and returns its corpus-relative (slash) path. The repro is a
// directory the revalidator's gate can run: a required repro.sh (offline execute)
// plus an optional prepare.sh (networked dep-warming). A drafter that cannot
// cover the record returns ErrUnsupported.
type Drafter interface {
	// Name identifies the implementation (e.g. "go-deprecation-template"); it is
	// recorded on the attached repro's label so the provenance is auditable.
	Name() string
	// Draft writes the candidate repro under root and returns its corpus-relative
	// path, or ErrUnsupported if it has no template for this record.
	Draft(ctx context.Context, root string, rec *record.Record) (string, error)
}
