// SPDX-License-Identifier: AGPL-3.0-only

package doctor

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
)

// Staleness is D2 (ADR-0001 §7, CONTEXT.md "stale"): it flags records whose
// applies_to no longer matches the live world. Report-only — it proposes a
// `stale` transition for human review, never mutates a record. It evaluates
// ONLY `validated` records — the served corpus; a quarantined draft cannot
// transition to stale, so the importer may ingest advisories for an EOL runtime
// (born quarantined) without this guard false-flagging them.
//
// Two signals, both fail-closed (no data ⇒ no flag):
//  1. provenance.valid.until is a date in the past (unambiguous; pure).
//  2. a Fixed version maps to an endoflife.date product cycle that is EOL.
//
// Signal 2 keys ONLY on Versions.Fixed (a version-bounded record, e.g. a vuln
// "fixed in X") and requires the version to match a real product cycle — so a
// deprecation record keyed on `introduced` (which persists) is never flagged,
// and a package version that isn't a runtime cycle simply finds no match.
//
// WouldFlag exposes the same two signals WITHOUT the validated-status gate — the
// promote-side mirror (#0071): the promoter calls it to refuse a born-stale
// advisory before it ever becomes validated, since once validated it would trip
// this very guard and red the validate PR.
type Staleness struct {
	eol      EOLSource
	now      time.Time
	products map[string]string  // lower-cased ecosystem → endoflife product
	cache    map[string][]Cycle // product → cycles, memoized across calls (callers are sequential)
}

// NewStaleness builds D2. eol may be nil (only the valid.until signal runs).
func NewStaleness(eol EOLSource, now time.Time) *Staleness {
	return &Staleness{
		eol: eol,
		now: now,
		products: map[string]string{
			"go":   "go",
			"npm":  "nodejs",
			"pypi": "python",
		},
		cache: map[string][]Cycle{},
	}
}

func (*Staleness) Name() string { return "staleness" }

var reMajorMinor = regexp.MustCompile(`^(\d+)(?:\.(\d+))?`)

// majorMinor reduces "2.15.0" → "2.15", "1.16" → "1.16", "18" → "18"; "" if no
// leading numeric version.
func majorMinor(v string) string {
	m := reMajorMinor.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return ""
	}
	if m[2] != "" {
		return m[1] + "." + m[2]
	}
	return m[1]
}

// isRuntimePackage reports whether a record's applies_to package denotes the
// language/runtime itself — the only case where a Fixed version is a runtime
// release cycle comparable to endoflife.date cycles. A third-party library's
// own version merely shares digits with a cycle by coincidence (kyverno v1.16
// vs Go 1.16), so signal 2 must not read it as one. Third-party Go modules are
// domain-qualified import paths (a host in the first segment, e.g.
// github.com/…, k8s.io/…); the runtime is the empty package or a bare,
// non-domain token (the stdlib import paths like io/ioutil, or "go").
func isRuntimePackage(pkg string) bool {
	p := strings.TrimSpace(pkg)
	if p == "" {
		return true // whole-ecosystem/runtime record
	}
	first := p
	if i := strings.IndexByte(p, '/'); i >= 0 {
		first = p[:i]
	}
	return !strings.Contains(first, ".")
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	return t, err == nil
}

func (s *Staleness) Run(ctx context.Context, recs []*record.Record) (Report, error) {
	rep := Report{Doctor: s.Name()}
	for _, r := range recs {
		// Staleness proposes a validated→stale demotion, so it evaluates ONLY
		// validated records. Quarantined drafts (incl. imported advisories that
		// target an EOL runtime — a draft is not "drift"), disputed, and
		// already-retired records are not candidates. This is what lets the importer
		// ingest EOL-runtime advisories without tripping the D2 guard test.
		if r.Status != "validated" {
			continue
		}
		if f := s.wouldFlag(ctx, r); f != nil {
			rep.Findings = append(rep.Findings, *f)
		}
	}
	return rep, nil
}

// WouldFlag reports the staleness finding a record WOULD receive if it were
// validated — independent of its current status. It is the promote-side mirror of
// the D2 guard (#0071, companion to #302): the promoter refuses a born-stale
// advisory (an EOL runtime, or a valid.until already past) with it, because such a
// record would be flagged the instant it became validated and red the guard test
// that gates the validate PR. nil = would not be flagged. The cycles cache is
// shared across calls (callers are sequential).
func (s *Staleness) WouldFlag(ctx context.Context, r *record.Record) *Finding {
	return s.wouldFlag(ctx, r)
}

// wouldFlag runs the two staleness signals with NO status gate (that lives in
// Run). One finding per record is enough to act on, so it returns on the first.
func (s *Staleness) wouldFlag(ctx context.Context, r *record.Record) *Finding {
	// Signal 1: an explicit validity window that has closed.
	if u := r.Provenance.Valid.Until; u != nil {
		if d, ok := parseDate(*u); ok && d.Before(s.now) {
			return &Finding{
				RecordID: r.ID, Path: r.Path,
				Issue:    fmt.Sprintf("provenance.valid.until %s is in the past", *u),
				Proposal: "confirm and set status: stale",
			}
		}
	}
	// Signal 2: a Fixed version on an EOL product cycle.
	if s.eol == nil {
		return nil
	}
	return s.staleByEOL(ctx, r)
}

func (s *Staleness) staleByEOL(ctx context.Context, r *record.Record) *Finding {
	for _, at := range r.AppliesTo {
		product := s.products[strings.ToLower(at.Ecosystem)]
		if product == "" || at.Versions == nil || at.Versions.Fixed == nil {
			continue // no confident mapping / no version-bound → skip
		}
		if !isRuntimePackage(at.Package) {
			continue // a third-party module's version is not a runtime cycle
		}
		cyc := majorMinor(*at.Versions.Fixed)
		if cyc == "" {
			continue
		}
		cycles, ok := s.cycles(ctx, product)
		if !ok {
			continue // skip on no data — never a false flag
		}
		for _, c := range cycles {
			if c.Cycle != cyc {
				continue
			}
			if d, ok := parseDate(c.EOL); ok && d.Before(s.now) {
				return &Finding{
					RecordID: r.ID, Path: r.Path,
					Issue:    fmt.Sprintf("%s %s reached end-of-life %s", product, cyc, c.EOL),
					Proposal: "confirm the affected versions are out of use and set status: stale",
				}
			}
			break // matched the cycle; not EOL → not stale
		}
	}
	return nil
}

// cycles returns the endoflife cycles for a product, memoized across calls so the
// promote gate (one WouldFlag per record) never re-fetches the same product. ok is
// false only when the source errored (caller skips — no data ⇒ no false flag); a
// 404/unknown product is a successful empty result and is cached. No lock: the only
// callers (Run's loop, the sequential promote loop) never touch one *Staleness
// concurrently.
func (s *Staleness) cycles(ctx context.Context, product string) ([]Cycle, bool) {
	if c, ok := s.cache[product]; ok {
		return c, true
	}
	c, err := s.eol.Cycles(ctx, product)
	if err != nil {
		return nil, false
	}
	s.cache[product] = c
	return c, true
}
