package idf

import "fmt"

// allowedLicenses is the hardcoded set of permissive SPDX license ids that
// data sources are allowed to carry.
var allowedLicenses = map[string]bool{
	"MIT":          true,
	"BSD-2-Clause": true,
	"BSD-3-Clause": true,
	"Apache-2.0":   true,
	"ISC":          true,
	"Python-2.0":   true,
	"Unlicense":    true,
	"CC-BY-4.0":    true,
}

// validateLicenses checks every source in the manifest against the
// hardcoded permissive license allowlist, returning an error naming the
// offending source's name and license if any source's license is not
// allowed.
func validateLicenses(m *Manifest) error {
	for _, src := range m.Sources {
		if !allowedLicenses[src.License] {
			return fmt.Errorf("source %q has disallowed license %q", src.Name, src.License)
		}
	}
	return nil
}
