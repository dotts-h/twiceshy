// SPDX-License-Identifier: AGPL-3.0-only

package telemetry

import (
	"path/filepath"
	"testing"
)

// The retro helpfulness join (#0069) hashes a session id with the standalone Hash so it
// does NOT need a write-Recorder. That hash MUST be byte-identical to the salted hash the
// serve-side Recorder stamps on the #0067 decision log — otherwise ServedInSession never
// matches the session and the join silently confirms nothing (the worst failure: a dead
// signal that looks alive). This is the parity the whole join depends on.
func TestHash_MatchesRecorderHash(t *testing.T) {
	salt := []byte("per-deployment-salt-9f3a")
	rec, err := NewRecorder(Config{Path: filepath.Join(t.TempDir(), "decisions.log"), Salt: salt})
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	defer func() { _ = rec.Close() }()

	for _, s := range []string{"sess-abc123", "another-session-id", "exp-0001", ""} {
		if got, want := Hash(salt, s), rec.Hash(s); got != want {
			t.Errorf("Hash(salt, %q) = %q, want Recorder.Hash = %q", s, got, want)
		}
	}
}

// Hash is deterministic, salt-sensitive, and the same 16-byte (32 hex char) width the
// Recorder emits.
func TestHash_DeterministicAndSaltSensitive(t *testing.T) {
	a := Hash([]byte("salt-a"), "session-x")
	if a != Hash([]byte("salt-a"), "session-x") {
		t.Error("Hash must be deterministic for the same salt + input")
	}
	if a == Hash([]byte("salt-b"), "session-x") {
		t.Error("different salts must produce different hashes (correlation must be deployment-scoped)")
	}
	if len(a) != 32 {
		t.Errorf("hash width = %d hex chars, want 32 (16 bytes)", len(a))
	}
}
