// SPDX-License-Identifier: AGPL-3.0-only

package record

import "strings"

var vulnIDPrefixes = []string{"GHSA-", "CVE-", "GO-"}

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
