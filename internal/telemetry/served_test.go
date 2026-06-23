// SPDX-License-Identifier: AGPL-3.0-only

package telemetry_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/telemetry"
)

// mustMarshalDecision builds a JSONL decision line for a session, padding `tokens` to
// at least padBytes so the marshalled line can be driven past a buffer threshold.
func mustMarshalDecision(t *testing.T, session, id string, padBytes int) string {
	t.Helper()
	d := telemetry.Decision{Channel: "search", Session: session, Served: []telemetry.ServedHit{{ID: id}}, Count: 1}
	chunk := strings.Repeat("x", 1024)
	for n := 0; n < padBytes; n += len(chunk) {
		d.Tokens = append(d.Tokens, chunk)
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	return string(b)
}

// A torn/garbled line is skipped (not fatal) and a valid line well past bufio's 64 KiB
// default is still read — the reader raises its scan buffer above the default, so a
// regression to the default would silently drop a long line and under-count the set.
func TestServedInSession_TornLineSkippedLongLineRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d.jsonl")
	sh := "deadbeefdeadbeef"
	content := mustMarshalDecision(t, sh, "exp-0001", 0) + "\n" +
		"{ this is not valid json\n" +
		mustMarshalDecision(t, sh, "exp-0002", 70*1024) + "\n" // ~70 KiB > the 64 KiB default
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	served, err := telemetry.ServedInSession(path, sh)
	if err != nil {
		t.Fatalf("a torn line must not be fatal: %v", err)
	}
	if !served["exp-0001"] {
		t.Errorf("the valid short line must be read despite the torn line: %v", served)
	}
	if !served["exp-0002"] {
		t.Errorf("a >64 KiB valid line must be read (buffer override): %v", served)
	}
}

// A line beyond the buffer cap (corruption — honest lines are tiny) is best-effort: it
// stops that file's scan but does NOT fail the join; lines before it are still read.
func TestServedInSession_OverlongLineNotFatal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d.jsonl")
	sh := "cafef00dcafef00d"
	content := mustMarshalDecision(t, sh, "exp-0001", 0) + "\n" +
		mustMarshalDecision(t, sh, "exp-0002", 1<<20+4096) + "\n" // > 1 MiB servedScanBuf
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	served, err := telemetry.ServedInSession(path, sh)
	if err != nil {
		t.Fatalf("an over-long line must not fail the read (best-effort): %v", err)
	}
	if !served["exp-0001"] {
		t.Errorf("the valid line before the over-long one must still be read: %v", served)
	}
}

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
