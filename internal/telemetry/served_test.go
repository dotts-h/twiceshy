// SPDX-License-Identifier: AGPL-3.0-only

package telemetry_test

import (
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/telemetry"
)

// ServedInSession returns the union of served ids attributed to one session (by its
// salted hash), so the retro helpfulness join can confirm only cards that were
// actually served in the session being judged (#0069). Other sessions' and
// session-less decisions are excluded.
func TestServedInSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.jsonl")
	rec, err := telemetry.NewRecorder(telemetry.Config{Path: path, Salt: []byte("s")})
	if err != nil {
		t.Fatal(err)
	}
	hashA := rec.Hash("session-A")
	hashB := rec.Hash("session-B")
	rec.Record(telemetry.Decision{Channel: "search", Session: hashA, Served: []telemetry.ServedHit{{ID: "exp-0001"}}})
	rec.Record(telemetry.Decision{Channel: "search", Session: hashA, Served: []telemetry.ServedHit{{ID: "exp-0002"}, {ID: "exp-0001"}}})
	rec.Record(telemetry.Decision{Channel: "push", Session: hashB, Served: []telemetry.ServedHit{{ID: "exp-0003"}}})
	rec.Record(telemetry.Decision{Channel: "search", Served: []telemetry.ServedHit{{ID: "exp-0099"}}}) // no session
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	served, err := telemetry.ServedInSession(path, hashA)
	if err != nil {
		t.Fatalf("ServedInSession: %v", err)
	}
	if len(served) != 2 || !served["exp-0001"] || !served["exp-0002"] {
		t.Fatalf("session A served set = %v, want {exp-0001, exp-0002}", served)
	}
	if served["exp-0003"] || served["exp-0099"] {
		t.Fatalf("session A must exclude other sessions' and session-less ids: %v", served)
	}

	if s, err := telemetry.ServedInSession(path, ""); err != nil || len(s) != 0 {
		t.Fatalf("an empty session hash must match nothing; got %v err %v", s, err)
	}
	if s, err := telemetry.ServedInSession(filepath.Join(dir, "absent.jsonl"), hashA); err != nil || len(s) != 0 {
		t.Fatalf("a missing log must be empty with no error; got %v err %v", s, err)
	}
}

// The reader unions across the one rotated generation (<path>.1) as well as the active
// log, so a session that spans a rotation is fully attributed.
func TestServedInSession_AcrossRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d.jsonl")
	rec, err := telemetry.NewRecorder(telemetry.Config{Path: path, Salt: []byte("s"), MaxBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	h := rec.Hash("sess")
	rec.Record(telemetry.Decision{Channel: "search", Session: h, Served: []telemetry.ServedHit{{ID: "exp-0001"}}})
	rec.Record(telemetry.Decision{Channel: "search", Session: h, Served: []telemetry.ServedHit{{ID: "exp-0002"}}})
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	served, err := telemetry.ServedInSession(path, h)
	if err != nil {
		t.Fatalf("ServedInSession: %v", err)
	}
	if len(served) != 2 || !served["exp-0001"] || !served["exp-0002"] {
		t.Fatalf("served across rotation = %v, want exp-0001 + exp-0002", served)
	}
}
