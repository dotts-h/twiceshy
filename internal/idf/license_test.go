package idf

import (
	"strings"
	"testing"
)

// TestValidateLicenses_AllAllowedPasses verifies validateLicenses returns a
// nil error when every source in the manifest carries an SPDX license id
// present on the hardcoded permissive allowlist.
func TestValidateLicenses_AllAllowedPasses(t *testing.T) {
	manifest := &Manifest{
		Sources: []ManifestSource{
			{Name: "nvd", Path: "/data/nvd", License: "MIT"},
			{Name: "osv", Path: "/data/osv", License: "Apache-2.0"},
			{Name: "cve", Path: "/data/cve", License: "CC-BY-4.0"},
		},
	}

	if err := validateLicenses(manifest); err != nil {
		t.Fatalf("validateLicenses(%+v) returned error %v, want nil", manifest, err)
	}
}

// TestValidateLicenses_DisallowedLicenseNamesOffendingSource verifies
// validateLicenses rejects a manifest containing a source whose license id
// is not on the allowlist, and that the returned error names the offending
// source's name and license so the failure is actionable.
func TestValidateLicenses_DisallowedLicenseNamesOffendingSource(t *testing.T) {
	manifest := &Manifest{
		Sources: []ManifestSource{
			{Name: "nvd", Path: "/data/nvd", License: "MIT"},
			{Name: "shady-feed", Path: "/data/shady", License: "GPL-3.0"},
		},
	}

	err := validateLicenses(manifest)
	if err == nil {
		t.Fatalf("validateLicenses(%+v) returned nil error, want error naming disallowed source", manifest)
	}

	msg := err.Error()
	if !strings.Contains(msg, "shady-feed") {
		t.Fatalf("validateLicenses error %q does not name offending source %q", msg, "shady-feed")
	}
	if !strings.Contains(msg, "GPL-3.0") {
		t.Fatalf("validateLicenses error %q does not name offending license %q", msg, "GPL-3.0")
	}
	if strings.Contains(msg, "nvd") {
		t.Fatalf("validateLicenses error %q unexpectedly names the allowed source %q", msg, "nvd")
	}
}
