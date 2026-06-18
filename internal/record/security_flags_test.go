// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// security_flags (#0011) documents an ingestion hazard. A quarantined record may
// carry flags; a validated record may NOT (a flagged record cannot be promoted).

func flaggedDraft(status string, flags []string) *record.Record {
	r := importerDraft() // from provenance_license_test.go (same package)
	r.Status = status
	r.Provenance.SecurityFlags = flags
	if status == "validated" {
		v := "2026-06-18"
		r.Provenance.ValidatedAt = &v
	}
	return r
}

func TestSecurityFlags_QuarantinedWithFlagsIsValid(t *testing.T) {
	r := flaggedDraft("quarantined", []string{"secret:aws-access-key"})
	if err := record.Validate(r); err != nil {
		t.Errorf("a quarantined record with security_flags must validate: %v", err)
	}
}

func TestSecurityFlags_ValidatedWithFlagsIsRejected(t *testing.T) {
	r := flaggedDraft("validated", []string{"harmful-code:pipe-to-shell"})
	err := record.Validate(r)
	if err == nil {
		t.Fatal("a validated record with security_flags must be rejected")
	}
	if !strings.Contains(err.Error(), "security_flags") {
		t.Errorf("error should mention security_flags, got: %v", err)
	}
}

func TestSecurityFlags_ValidatedWithoutFlagsIsValid(t *testing.T) {
	r := flaggedDraft("validated", nil)
	if err := record.Validate(r); err != nil {
		t.Errorf("a validated record without flags must validate: %v", err)
	}
}

func TestSecurityFlags_SatisfyJSONSchema(t *testing.T) {
	schema := loadRecordSchema(t) // schema_test.go (same package)
	r := flaggedDraft("quarantined", []string{"secret:aws-access-key", "harmful-code:pipe-to-shell"})
	out, err := record.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := schema.Validate(frontmatterValue(t, out)); err != nil {
		t.Errorf("quarantined record with security_flags must satisfy schema: %v\n--- marshaled ---\n%s", err, out)
	}
}
