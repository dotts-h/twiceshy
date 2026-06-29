// SPDX-License-Identifier: AGPL-3.0-only

package main

import "testing"

// The serve and the retro join MUST agree on the salt or the #0069 session hashes
// diverge (the #0098 bug). This pins the shared rule: explicit salt wins, else the
// bearer token is the fallback (NOT empty — an empty fallback is what broke the join).
func TestTelemetrySalt(t *testing.T) {
	for _, tc := range []struct {
		name, explicit, token, want string
	}{
		{"explicit salt wins over the token", "explicit-salt", "tok", "explicit-salt"},
		{"empty salt falls back to the bearer token", "", "tok", "tok"},
		{"explicit salt used even when token is empty", "s", "", "s"},
		{"both empty is empty (no telemetry secret)", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := telemetrySalt(tc.explicit, tc.token); got != tc.want {
				t.Errorf("telemetrySalt(%q, %q) = %q, want %q", tc.explicit, tc.token, got, tc.want)
			}
		})
	}
}
