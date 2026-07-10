// SPDX-License-Identifier: AGPL-3.0-only

package record

import (
	"regexp"
	"strings"
)

var vulnIDPrefixes = []string{"GHSA-", "CVE-", "GO-"}

// vulnIDPattern matches the GHSA/CVE/GO advisory id shapes anywhere in a string
// (e.g. inside a source_url path). Case-insensitive; GHSA segment lengths vary, so
// the middle/tail segments are length-agnostic.
var vulnIDPattern = regexp.MustCompile(`(?i)(GHSA-[0-9a-z]{4,}-[0-9a-z]{4,}-[0-9a-z]{4,}|CVE-\d{4}-\d+|GO-\d{4}-\d+)`)

// fixTextClaimsFixedVersion reports whether a remediation string promises an
// upgrade past a fixed version — the boilerplate that contradicts an advisory with
// no published fix (osvLiveFixText emits "No fix is published yet" in that case).
func fixTextClaimsFixedVersion(fix string) bool {
	return strings.Contains(strings.ToLower(fix), "past the fixed version")
}

// AdvisoryDefects returns deterministic transcription-defect flags for an
// advisory-class record — the #0061 defect classes a rule-based gate catches
// WITHOUT an LLM (defense in depth: the judge is no longer the sole gate, and the
// same detector re-normalizes the legacy backlog). It returns nil for a clean or a
// non-advisory record. Each flag is a stable "consistency:<class>[:detail]" string,
// mirroring the security_flags vocabulary so a flagged record cannot be promoted.
//
// Detection is conservative — it fires only on unambiguous internal defects, never
// on something it merely cannot verify (a terse-but-faithful advisory PASSES):
//   - null-fixed-fix-text: the fix text promises an upgrade past a fixed version
//     while NO affected range carries one (exp-0061's class).
//   - source-url-id-mismatch: a GHSA/CVE/GO id embedded in the source_url is one the
//     record does not carry in its error_signatures (the URL points elsewhere).
//   - malformed-package-path: a package coordinate with a scheme prefix or
//     whitespace — never a valid module/package path.
//   - audited source facts: the four semantic #0061 cases whose canonical scope
//     was independently established (libheif, Prometheus, Traefik, Ech0). These
//     are exact advisory+ecosystem+package matches, not unsafe generic casing or
//     semantic-import-version heuristics.
func AdvisoryDefects(rec *Record) []string {
	if !IsAdvisoryClass(rec) {
		return nil
	}
	var defects []string
	ownIDs := recordVulnIDs(rec)

	if rec.Resolution != nil && fixTextClaimsFixedVersion(rec.Resolution.Fix) && !anyFixedVersion(rec.AppliesTo) {
		defects = append(defects, "consistency:null-fixed-fix-text")
	}

	if mismatch := mismatchedURLVulnID(rec); mismatch != "" {
		defects = append(defects, "consistency:source-url-id-mismatch:"+mismatch)
	}

	for _, a := range rec.AppliesTo {
		if malformedPackagePath(a.Package) {
			defects = append(defects, "consistency:malformed-package-path:"+strings.TrimSpace(a.Package))
		}
		for _, known := range knownAdvisoryScopeDefects {
			if ownIDs[known.advisoryID] && strings.EqualFold(strings.TrimSpace(a.Ecosystem), known.ecosystem) && strings.TrimSpace(a.Package) == known.pkg {
				defects = appendUnique(defects, known.flag+":"+known.pkg)
			}
		}
	}

	return defects
}

// blockingConsistencyPrefixes are the deterministic transcription defects precise
// enough to HARD-block promotion. Source-url comparison is alias-aware: recordVulnIDs
// includes every primary id and alias carried by the record, so a mismatch means the
// URL names an advisory absent from that complete set.
var blockingConsistencyPrefixes = []string{
	"consistency:null-fixed-fix-text",
	"consistency:malformed-package-path",
	"consistency:source-url-id-mismatch",
	"consistency:ecosystem-package-mismatch",
	"consistency:go-major-version-path",
	"consistency:go-module-path-case",
}

// IsBlockingConsistencyFlag reports whether a consistency flag (stored or freshly
// detected) is in a promotion-blocking class.
func IsBlockingConsistencyFlag(flag string) bool {
	for _, p := range blockingConsistencyPrefixes {
		if strings.HasPrefix(flag, p) {
			return true
		}
	}
	return false
}

// AdvisoryBlockingDefects returns the live-detected #0061 defects that HARD-block
// promotion. It is the promote-path pre-gate: a legacy record with no stored flag
// (ingested before the consistency gate existed) is still held when its content
// trips a deterministic structural or exact audited-source rule.
func AdvisoryBlockingDefects(rec *Record) []string {
	var blocking []string
	for _, f := range AdvisoryDefects(rec) {
		if IsBlockingConsistencyFlag(f) {
			blocking = append(blocking, f)
		}
	}
	return blocking
}

// anyFixedVersion reports whether any affected range carries a non-empty fixed version.
func anyFixedVersion(applies []AppliesTo) bool {
	for _, a := range applies {
		if a.Versions != nil && a.Versions.Fixed != nil && strings.TrimSpace(*a.Versions.Fixed) != "" {
			return true
		}
	}
	return false
}

// mismatchedURLVulnID returns a vuln id found in the record's source_url that the
// record itself does NOT carry (in error_signatures/fingerprints), or "" if the URL
// is consistent. A URL with no recognizable advisory id is not a mismatch (we only
// flag a concrete, recognizable id pointing at a different advisory).
func mismatchedURLVulnID(rec *Record) string {
	if rec == nil {
		return ""
	}
	own := recordVulnIDs(rec)
	if len(own) == 0 {
		return ""
	}
	for _, id := range vulnIDPattern.FindAllString(rec.Provenance.SourceURL, -1) {
		if !own[strings.ToUpper(id)] {
			return id
		}
	}
	return ""
}

// recordVulnIDs is the set (upper-cased) of advisory ids the record carries in its
// error_signatures and fingerprints.
func recordVulnIDs(rec *Record) map[string]bool {
	ids := map[string]bool{}
	if rec.Symptom == nil {
		return ids
	}
	add := func(s string) {
		if hasVulnIDPrefix(s) {
			ids[strings.ToUpper(strings.TrimSpace(s))] = true
		}
	}
	for _, sig := range rec.Symptom.ErrorSignatures {
		add(sig)
	}
	if fp := rec.Symptom.Fingerprints; fp != nil {
		for _, s := range append(append([]string{}, fp.App...), fp.Generic...) {
			add(s)
		}
	}
	return ids
}

// knownAdvisoryScopeDefects is deliberately narrow. Each entry is one concrete
// source transcription rejected by the full #0061 audit. Go module casing and /vN
// suffixes cannot be inferred safely from strings alone (Azure legitimately uses
// uppercase; Prometheus v2 releases legitimately use an unsuffixed module), so do
// not generalize this table into a normalizer. New facts require their own evidence.
var knownAdvisoryScopeDefects = []struct {
	advisoryID string
	ecosystem  string
	pkg        string
	flag       string
}{
	{"GHSA-22FX-6R9M-R8H9", "Go", "github.com/strukturag/libheif", "consistency:ecosystem-package-mismatch"},
	{"GHSA-4V48-4Q5M-8VX4", "Go", "github.com/prometheus/prometheus/v2", "consistency:go-major-version-path"},
	{"GHSA-7V4P-328V-8V5G", "Go", "github.com/traefik/traefik", "consistency:go-major-version-path"},
	{"GHSA-FPW6-HRG5-Q5X5", "Go", "github.com/lin-snow/Ech0", "consistency:go-module-path-case"},
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// malformedPackagePath reports whether a package coordinate is structurally invalid:
// it carries a URL scheme (https://, http://) or embedded whitespace. Conservative
// by design — uppercase or fabricated major-version suffixes are ecosystem-specific
// judgement calls left to the panel, not this rule-based gate.
func malformedPackagePath(pkg string) bool {
	if strings.TrimSpace(pkg) == "" {
		return false
	}
	if strings.Contains(pkg, "://") {
		return true
	}
	return strings.ContainsAny(pkg, " \t\n")
}

// IsAdvisoryClass reports whether a record is an externally-imported vulnerability
// advisory (ADR-0016 §1): it carries a vulnerability identifier (GHSA-/CVE-/GO-
// prefix) in symptom.error_signatures or fingerprints. Deprecation/codemod records
// are imported too but carry NO vuln id, so they are NOT advisory-class — they stay
// on the proof+judge path.
func IsAdvisoryClass(rec *Record) bool {
	if rec == nil {
		return false
	}
	if rec.Symptom != nil {
		for _, sig := range rec.Symptom.ErrorSignatures {
			if hasVulnIDPrefix(sig) {
				return true
			}
		}
		if fp := rec.Symptom.Fingerprints; fp != nil {
			for _, s := range append(append([]string{}, fp.App...), fp.Generic...) {
				if hasVulnIDPrefix(s) {
					return true
				}
			}
		}
	}
	return false
}

// IsProseClass reports whether a record is a pure-prose lesson (ADR-0020): NOT
// advisory-class (no vuln id) AND NOT execution-provable (no positive repro). Such a
// record routes to neither the ADR-0013 §1 proof+judge path nor the ADR-0016 advisory
// panel — it is the prose class the ADR-0020 cross-family panel promotes. A nil record is
// not prose-class.
func IsProseClass(rec *Record) bool {
	if rec == nil {
		return false
	}
	return !IsAdvisoryClass(rec) && !HasPositiveRepro(rec)
}

func hasVulnIDPrefix(s string) bool {
	up := strings.ToUpper(strings.TrimSpace(s))
	for _, p := range vulnIDPrefixes {
		if strings.HasPrefix(up, p) {
			return true
		}
	}
	return false
}
