// SPDX-License-Identifier: AGPL-3.0-only

package fingerprint

// DedupResult holds the classification of derived fingerprints into those
// that are new (not present in the known set) and those that already exist.
type DedupResult struct {
	New      map[string]string // fingerprint -> scope, derived but NOT already present
	Existing map[string]string // fingerprint -> scope, derived AND already present
}

// Dedup derives generic and app-scoped fingerprints for each signature in sigs,
// deduplicates them by fingerprint value, and classifies each as new or existing
// based on membership in the known set. Both result maps are always non-nil.
func Dedup(repo string, sigs []string, known map[string]bool) DedupResult {
	result := DedupResult{
		New:      make(map[string]string),
		Existing: make(map[string]string),
	}
	seen := make(map[string]bool) // tracks fingerprints already processed in this call

	for _, sig := range sigs {
		genericFP := Generic(sig)
		if !seen[genericFP] {
			seen[genericFP] = true
			if known[genericFP] {
				result.Existing[genericFP] = "generic"
			} else {
				result.New[genericFP] = "generic"
			}
		}

		appFP := App(repo, sig)
		if !seen[appFP] {
			seen[appFP] = true
			if known[appFP] {
				result.Existing[appFP] = "app"
			} else {
				result.New[appFP] = "app"
			}
		}
	}

	return result
}
