// SPDX-License-Identifier: AGPL-3.0-only

package main

// telemetrySalt resolves the salt for the #0067 gate-decision telemetry hashes
// (query_hash and the #0069 session-correlation key). The SERVE (which writes the
// log) and the retro-intake JOIN (which reads it) MUST resolve this identically, or
// their session hashes diverge and the served->used helpfulness join silently
// attributes nothing — a real prod bug (#0098): the serve fell back to the bearer
// token while the drain used an empty salt, so 0 of N sessions ever correlated.
//
// Rule: an explicit TWICESHY_TELEMETRY_SALT wins; otherwise fall back to the
// per-deployment bearer token (a secret that's already shared across the
// deployment). Centralised here so the two call sites can't drift apart again.
func telemetrySalt(explicit, token string) string {
	if explicit != "" {
		return explicit
	}
	return token
}
