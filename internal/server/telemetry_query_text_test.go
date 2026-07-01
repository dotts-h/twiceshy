// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/telemetry"
)

// readRawLine returns the single JSONL line written to path, failing the test if
// there isn't exactly one.
func readRawLine(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("want exactly 1 decision line, got %d: %q", len(lines), raw)
	}
	return lines[0]
}

// readLines decodes every JSONL line at path into a telemetry.Decision.
func readLines(t *testing.T, path string) []telemetry.Decision {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	var out []telemetry.Decision
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var d telemetry.Decision
		if err := json.Unmarshal(sc.Bytes(), &d); err != nil {
			t.Fatalf("decode %q: %v", sc.Text(), err)
		}
		out = append(out, d)
	}
	return out
}

// TestRecordPushDecision_QueryTextFlagOff pins the #0109 default: with the flag
// off (the handlers zero value), the gate-decision line must have NO query_text
// key at all — byte-behavior identical to before #0109 — even though the query
// text is available to the handler.
func TestRecordPushDecision_QueryTextFlagOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	tel, err := telemetry.NewRecorder(telemetry.Config{Path: path, Salt: []byte("salt")})
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{telemetry: tel} // queryText defaults false
	h.recordPushDecision("a raw prompt nobody opted in to persist", index.PushDecision{}, "")
	if err := tel.Close(); err != nil {
		t.Fatal(err)
	}

	line := readRawLine(t, path)
	if strings.Contains(line, "query_text") {
		t.Fatalf("flag off must omit query_text entirely: %s", line)
	}
}

// TestRecordPushDecision_QueryTextFlagOnTruncates pins the #0109 opt-in path: the
// query hash is always present, and query text is captured but truncated to at most
// 256 bytes at a UTF-8 rune boundary so a multibyte rune straddling the cut can never
// split into invalid bytes (which would JSON-escape as replacement runes).
func TestRecordPushDecision_QueryTextFlagOnTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	tel, err := telemetry.NewRecorder(telemetry.Config{Path: path, Salt: []byte("salt")})
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{telemetry: tel, queryText: true}
	// 255 ASCII bytes + a 3-byte rune (€) straddling byte 256: a naive [:256] slice
	// would split the rune's bytes.
	long := strings.Repeat("a", 255) + "€€€€"
	h.recordPushDecision(long, index.PushDecision{}, "")
	if err := tel.Close(); err != nil {
		t.Fatal(err)
	}

	got := readLines(t, path)
	if len(got) != 1 {
		t.Fatalf("want 1 decision, got %d", len(got))
	}
	d := got[0]
	if d.QueryHash == "" {
		t.Fatal("query hash must always be present, flag on or off")
	}
	if len(d.QueryText) > 256 {
		t.Fatalf("query_text not truncated: %d bytes", len(d.QueryText))
	}
	if !utf8.ValidString(d.QueryText) {
		t.Fatalf("truncated query_text is not valid UTF-8: %q", d.QueryText)
	}
	if !strings.HasPrefix(long, d.QueryText) {
		t.Fatalf("query_text must be a clean prefix of the original query: %q", d.QueryText)
	}
}

// TestRecordSearchDecision_QueryTextFlagOff mirrors the push test for the search
// channel — both channels get the field (#0109).
func TestRecordSearchDecision_QueryTextFlagOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	tel, err := telemetry.NewRecorder(telemetry.Config{Path: path, Salt: []byte("salt")})
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{telemetry: tel}
	h.recordSearchDecision("a raw prompt nobody opted in to persist", nil, "")
	if err := tel.Close(); err != nil {
		t.Fatal(err)
	}

	line := readRawLine(t, path)
	if strings.Contains(line, "query_text") {
		t.Fatalf("flag off must omit query_text entirely: %s", line)
	}
}

func TestRecordSearchDecision_QueryTextFlagOnTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	tel, err := telemetry.NewRecorder(telemetry.Config{Path: path, Salt: []byte("salt")})
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{telemetry: tel, queryText: true}
	long := strings.Repeat("b", 255) + "€€€€"
	h.recordSearchDecision(long, nil, "")
	if err := tel.Close(); err != nil {
		t.Fatal(err)
	}

	got := readLines(t, path)
	if len(got) != 1 {
		t.Fatalf("want 1 decision, got %d", len(got))
	}
	d := got[0]
	if d.QueryHash == "" {
		t.Fatal("query hash must always be present, flag on or off")
	}
	if len(d.QueryText) > 256 {
		t.Fatalf("query_text not truncated: %d bytes", len(d.QueryText))
	}
	if !utf8.ValidString(d.QueryText) {
		t.Fatalf("truncated query_text is not valid UTF-8: %q", d.QueryText)
	}
}
