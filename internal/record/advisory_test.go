// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

func TestIsAdvisoryClass_GHSARecord(t *testing.T) {
	// Mirrors exp-0007 (GHSA advisory).
	rec := &record.Record{
		Symptom: &record.Symptom{
			ErrorSignatures: []string{"GHSA-227x-7mh8-3cf6", "CVE-2025-59823", "GO-2025-3981"},
		},
	}
	if !record.IsAdvisoryClass(rec) {
		t.Fatal("exp-0007 shape (GHSA/CVE/GO ids) must be advisory-class")
	}
}

func TestIsAdvisoryClass_DeprecationRecordFalse(t *testing.T) {
	rec := &record.Record{
		Symptom: &record.Symptom{
			Summary: "strings.Title deprecated",
			ErrorSignatures: []string{
				"SA1019: strings.Title is deprecated: The rule Title uses for word boundaries does not handle Unicode punctuation properly.",
			},
		},
	}
	if record.IsAdvisoryClass(rec) {
		t.Fatal("exp-0044 shape (deprecation, no vuln id) must NOT be advisory-class")
	}
}

func TestIsAdvisoryClass_ProseRecordFalse(t *testing.T) {
	rec := &record.Record{
		Symptom: &record.Symptom{Summary: "the handler panics on nil input"},
	}
	if record.IsAdvisoryClass(rec) {
		t.Fatal("plain prose record must NOT be advisory-class")
	}
}

func TestIsAdvisoryClass_FingerprintMatch(t *testing.T) {
	rec := &record.Record{
		Symptom: &record.Symptom{
			Fingerprints: &record.Fingerprints{Generic: []string{"cve-2024-1234"}},
		},
	}
	if !record.IsAdvisoryClass(rec) {
		t.Fatal("vuln id in fingerprints.generic must be advisory-class")
	}
}
