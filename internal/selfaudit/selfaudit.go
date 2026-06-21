// SPDX-License-Identifier: AGPL-3.0-only

// Package selfaudit dogfoods twiceshy on its own dependencies (#0014): it matches
// the modules in twiceshy's go.mod against the vulnerability advisories the
// importer has ingested (#0007) and reports any dependency a Go-ecosystem
// advisory flags as affected at its current version. Using the product on itself.
package selfaudit

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dotts-h/twiceshy/internal/record"
)

// Dep is one required module and its version, as written in go.mod.
type Dep struct {
	Path    string
	Version string
}

// Hit is a dependency that an advisory flags as affected at its current version.
type Hit struct {
	Dep        Dep
	RecordID   string
	AdvisoryID string // the GHSA-/CVE-/GO- identifier the record carries
	Introduced string
	Fixed      string // "" when no fix is published
}

// ParseGoMod extracts the required modules (direct AND indirect — a vulnerability
// hides just as easily in a transitive dependency) from a go.mod. It deliberately
// hand-parses rather than pull in golang.org/x/mod/modfile, keeping the dependency
// budget (CONVENTIONS) intact. module/go/replace/exclude/retract/toolchain
// directives are skipped, so neither the module's own path nor a replace RHS is
// mistaken for a require.
func ParseGoMod(r io.Reader) ([]Dep, error) {
	var deps []Dep
	sc := bufio.NewScanner(r)
	inRequire := false
	for sc.Scan() {
		line := strings.TrimSpace(stripComment(sc.Text()))
		if line == "" {
			continue
		}
		if inRequire {
			if line == ")" {
				inRequire = false
				continue
			}
			if d, ok := parseRequire(line); ok {
				deps = append(deps, d)
			}
			continue
		}
		switch {
		case line == "require (":
			inRequire = true
		case strings.HasPrefix(line, "require "):
			if d, ok := parseRequire(strings.TrimPrefix(line, "require ")); ok {
				deps = append(deps, d)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading go.mod: %w", err)
	}
	return deps, nil
}

func parseRequire(s string) (Dep, bool) {
	f := strings.Fields(s)
	if len(f) < 2 {
		return Dep{}, false
	}
	return Dep{Path: f[0], Version: f[1]}, true
}

func stripComment(s string) string {
	if i := strings.Index(s, "//"); i >= 0 {
		return s[:i]
	}
	return s
}

// Audit reports every dependency that a Go-ecosystem vulnerability advisory in
// recs flags as affected. A record is a security signal only if it carries a
// vulnerability id (see vulnPrefixes) — a deprecation record that happens to
// match a package is not a vulnerability and is ignored.
func Audit(deps []Dep, recs []*record.Record) []Hit {
	var hits []Hit
	for _, rec := range recs {
		vid := advisoryID(rec)
		if vid == "" {
			continue // not a vulnerability advisory — deprecations carry no vuln id
		}
		for _, at := range rec.AppliesTo {
			if !strings.EqualFold(at.Ecosystem, "Go") {
				continue
			}
			for _, d := range deps {
				if d.Path == at.Package && affected(d.Version, at.Versions) {
					intro, fixed := rangeStr(at.Versions)
					hits = append(hits, Hit{Dep: d, RecordID: rec.ID, AdvisoryID: vid, Introduced: intro, Fixed: fixed})
				}
			}
		}
	}
	return hits
}

// affected reports whether version v falls in the advisory's affected range:
// introduced <= v < fixed. A nil range (or empty introduced) has no lower bound;
// a nil/empty fixed means no fix is published, so every version >= introduced is
// affected (the OSV convention #0062 also relies on).
func affected(v string, vr *record.VersionRange) bool {
	if vr == nil {
		return true
	}
	if vr.Introduced != nil && *vr.Introduced != "" && cmpVer(v, *vr.Introduced) < 0 {
		return false
	}
	if vr.Fixed != nil && *vr.Fixed != "" && cmpVer(v, *vr.Fixed) >= 0 {
		return false
	}
	return true
}

// vulnPrefixes are the OSV vulnerability-id prefixes the corpus carries (GHSA,
// CVE, GO, MAL malicious-package, RUSTSEC). This is deliberately BROADER than
// record.IsAdvisoryClass (GHSA/CVE/GO only, ADR-0016 §1, which gates judge-panel
// routing): a dependency security monitor that skipped MAL- would miss a
// malicious package in its own deps — the worst possible miss.
var vulnPrefixes = []string{"GHSA-", "CVE-", "GO-", "MAL-", "RUSTSEC-"}

// advisoryID returns the first vulnerability identifier (a vulnPrefixes match) in
// the record's error signatures, for the report. An empty result means the record
// is not a vulnerability advisory, so Audit skips it.
func advisoryID(rec *record.Record) string {
	if rec.Symptom == nil {
		return ""
	}
	for _, sig := range rec.Symptom.ErrorSignatures {
		up := strings.ToUpper(strings.TrimSpace(sig))
		for _, p := range vulnPrefixes {
			if strings.HasPrefix(up, p) {
				return strings.TrimSpace(sig)
			}
		}
	}
	return ""
}

func rangeStr(vr *record.VersionRange) (introduced, fixed string) {
	if vr == nil {
		return "", ""
	}
	if vr.Introduced != nil {
		introduced = *vr.Introduced
	}
	if vr.Fixed != nil {
		fixed = *vr.Fixed
	}
	return introduced, fixed
}

// cmpVer compares two version strings (a leading "v" is optional), returning
// -1, 0, or +1. It compares the dotted numeric core, and — per semver — a
// pre-release (a "-suffix") sorts BEFORE the same release, while build metadata
// (a "+suffix") is ignored. The pre-release rule matters at the fixed boundary:
// without it, a dependency pinned to vX.Y.Z-rc would be mistaken for the fix vX.Y.Z
// and a real vuln missed — the fail-UNSAFE direction a security monitor must not
// take. This covers Go-module semver and the OSV convention where introduced "0"
// means "from the first version ever"; exotic forms (pseudo-versions) compare on
// their numeric prefix only — a documented limitation.
func cmpVer(a, b string) int {
	an, apre := parseVer(a)
	bn, bpre := parseVer(b)
	for i := 0; i < len(an) || i < len(bn); i++ {
		var x, y int
		if i < len(an) {
			x = an[i]
		}
		if i < len(bn) {
			y = bn[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	// Equal numeric core: a pre-release is lower precedence than the release.
	switch {
	case apre && !bpre:
		return -1
	case !apre && bpre:
		return 1
	default:
		return 0
	}
}

// parseVer splits a version into its numeric core and whether it carries a
// pre-release suffix. A leading "v" is dropped; "+build" metadata is stripped
// (no precedence effect); a "-pre" suffix sets hasPre and is excluded from the
// numeric core.
func parseVer(v string) (parts []int, hasPre bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i] // build metadata: ignored in precedence
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		hasPre = true
		v = v[:i] // pre-release: lowers precedence
	}
	if v == "" {
		return nil, hasPre
	}
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			break // non-numeric component (e.g. a pseudo-version date) — match on the numeric prefix
		}
		parts = append(parts, n)
	}
	return parts, hasPre
}
