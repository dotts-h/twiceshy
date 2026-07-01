// SPDX-License-Identifier: AGPL-3.0-only

package telemetry_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/telemetry"
)

func fixedNow() time.Time { return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC) }

func newRec(t *testing.T, path string, maxBytes int64) *telemetry.Recorder {
	t.Helper()
	r, err := telemetry.NewRecorder(telemetry.Config{Path: path, MaxBytes: maxBytes, Salt: []byte("pepper"), Now: fixedNow})
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func readLines(t *testing.T, path string) []telemetry.Decision {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer func() { _ = f.Close() }()
	var out []telemetry.Decision
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var d telemetry.Decision
		if err := json.Unmarshal(sc.Bytes(), &d); err != nil {
			t.Fatalf("decode line %q: %v", sc.Text(), err)
		}
		out = append(out, d)
	}
	return out
}

// readLinesIfExists is like readLines but treats a missing file as zero lines,
// so callers can recover decisions across both rotation generations (the active
// <path> and the rotated <path>.1) without caring whether a rotation occurred.
func readLinesIfExists(t *testing.T, path string) []telemetry.Decision {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return readLines(t, path)
}

// readAllGenerations recovers every decision still on disk across both retained
// generations. The reader's json.Unmarshal rejects any malformed or interleaved
// line, so a torn write under concurrency would fail the test here.
func readAllGenerations(t *testing.T, path string) []telemetry.Decision {
	t.Helper()
	out := readLinesIfExists(t, path)
	out = append(out, readLinesIfExists(t, path+".1")...)
	return out
}

func TestRecorder_WritesJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	r := newRec(t, path, 1<<20)
	r.Record(telemetry.Decision{
		Channel: "push", QueryHash: r.Hash("modernc.org/sqlite busy_timeout"),
		Tokens: []string{"sqlite", "busy_timeout"}, Served: []telemetry.ServedHit{{ID: "exp-0001", Score: 1.5}}, Count: 1,
	})
	r.Record(telemetry.Decision{Channel: "search", QueryHash: r.Hash("other"), Count: 0})
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got := readLines(t, path)
	if len(got) != 2 {
		t.Fatalf("want 2 decision lines, got %d", len(got))
	}
	first := got[0]
	if first.Channel != "push" || first.Count != 1 || len(first.Served) != 1 || first.Served[0].ID != "exp-0001" {
		t.Errorf("first decision wrong: %+v", first)
	}
	if first.Time != "2026-06-21T12:00:00Z" {
		t.Errorf("recorder must stamp the time; got %q", first.Time)
	}
}

func TestRecorder_CreatesLogOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	r := newRec(t, path, 1<<20)
	r.Record(telemetry.Decision{Channel: "search", QueryHash: r.Hash("private query"), Count: 1})
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("telemetry log permissions = %o, want 600", got)
	}
}

func TestRecorder_HashRedactsQuery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	r := newRec(t, path, 1<<20)
	const secret = "my secret prompt about github.com/acme/private"
	h := r.Hash(secret)
	if h == "" || h == secret {
		t.Fatalf("hash must be non-empty and not the plaintext: %q", h)
	}
	if r.Hash(secret) != h {
		t.Error("hash must be deterministic for the same salt")
	}
	// A different salt yields a different hash (no cross-deployment correlation).
	other, _ := telemetry.NewRecorder(telemetry.Config{Path: path, Salt: []byte("different"), Now: fixedNow})
	defer func() { _ = other.Close() }()
	if other.Hash(secret) == h {
		t.Error("hash must depend on the salt")
	}
	r.Record(telemetry.Decision{Channel: "search", QueryHash: h})
	_ = r.Close()
	for _, d := range readLines(t, path) {
		// The raw prompt text must never reach disk.
		if d.QueryHash == secret {
			t.Fatal("raw query must not be persisted")
		}
	}
}

// Decision.QueryText is the #0109 opt-in raw-query capture: the telemetry package
// itself stays dumb (it serializes whatever it's given), so this tests the JSON
// contract directly rather than the caller's flag — the field must vanish from the
// line entirely when unset (byte-behavior identical to before #0109) and appear
// verbatim when the caller populates it.
func TestDecision_QueryTextOmittedWhenUnset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	r := newRec(t, path, 1<<20)
	r.Record(telemetry.Decision{Channel: "push", QueryHash: r.Hash("q")})
	_ = r.Close()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "query_text") {
		t.Fatalf("query_text must be absent from the line when unset: %s", raw)
	}
}

func TestDecision_QueryTextPresentWhenSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	r := newRec(t, path, 1<<20)
	r.Record(telemetry.Decision{Channel: "push", QueryHash: r.Hash("q"), QueryText: "modernc.org/sqlite busy_timeout"})
	_ = r.Close()

	got := readLines(t, path)
	if len(got) != 1 || got[0].QueryText != "modernc.org/sqlite busy_timeout" {
		t.Fatalf("query_text not round-tripped: %+v", got)
	}
}

// When the rotate rename fails persistently, telemetry must STOP (the documented
// "on any failure sets r.f = nil") rather than reset size and keep appending to the
// un-rotated file, which would silently break the ~2*MaxBytes on-disk bound.
func TestRecorder_StopsOnRotateRenameFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d.jsonl")
	r := newRec(t, path, 256) // tiny cap forces rotation
	// Pre-create <path>.1 as a directory so os.Rename(path, path+".1") fails while
	// a reopen of path would still succeed.
	if err := os.Mkdir(path+".1", 0o755); err != nil {
		t.Fatalf("mkdir blocker: %v", err)
	}
	for i := 0; i < 50; i++ {
		r.Record(telemetry.Decision{Channel: "push", QueryHash: "h", Tokens: []string{"tok", "tok", "tok"}, Count: i})
	}
	_ = r.Close()

	cur, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat current: %v", err)
	}
	if cur.Size() > 256*3 {
		t.Errorf("active log unbounded after a failed rotate rename: %d bytes (cap 256)", cur.Size())
	}
}

func TestRecorder_RotatesAtCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	r := newRec(t, path, 256) // tiny cap forces rotation
	for i := 0; i < 50; i++ {
		r.Record(telemetry.Decision{Channel: "push", QueryHash: "h", Tokens: []string{"tok", "tok", "tok"}, Count: i})
	}
	_ = r.Close()

	cur, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat current: %v", err)
	}
	// The active file is bounded near the cap (one record may push it just over).
	if cur.Size() > 256*3 {
		t.Errorf("active log unbounded: %d bytes (cap 256)", cur.Size())
	}
	// The rotated generation exists, so old decisions are retained, not lost.
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("expected a rotated generation %s.1: %v", path, err)
	}
}

func TestRecorder_NilAndConcurrent(t *testing.T) {
	var nilRec *telemetry.Recorder
	nilRec.Record(telemetry.Decision{Channel: "push"}) // must not panic
	if err := nilRec.Close(); err != nil {
		t.Errorf("nil Close: %v", err)
	}

	path := filepath.Join(t.TempDir(), "d.jsonl")
	r := newRec(t, path, 1<<20)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r.Record(telemetry.Decision{Channel: "push", QueryHash: "h", Count: n})
		}(i)
	}
	wg.Wait()
	_ = r.Close()
	if got := readLines(t, path); len(got) != 100 {
		t.Fatalf("concurrent records corrupted the log: want 100 clean lines, got %d", len(got))
	}
}

// TestRecorder_RotatesUnderConcurrency crosses a rotation boundary while many
// goroutines record at once: the single writer goroutine must keep each JSONL
// line intact (no interleaving/torn writes — the reader's json.Unmarshal would
// reject a corrupt line) and the on-disk decisions plus the drop counter must
// account for no more than the sent total. Strict equality is intentionally NOT
// asserted: rotate() keeps only one prior generation (os.Rename overwrites .1),
// so multiple rotations legitimately discard older data — see decision.go.
func TestRecorder_RotatesUnderConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	// Tiny cap + a roomy buffer: every record is padded so a handful of bytes
	// triggers many rotations, but the buffer is large enough that writes land on
	// disk (recovered > 0) rather than all dropping.
	r, err := telemetry.NewRecorder(telemetry.Config{
		Path: path, MaxBytes: 256, Buffer: 4096, Salt: []byte("pepper"), Now: fixedNow,
	})
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}

	const n = 400
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(c int) {
			defer wg.Done()
			// Padding forces several rotations across the run.
			r.Record(telemetry.Decision{
				Channel:   "push",
				QueryHash: "h",
				Tokens:    []string{"tok", "tok", "tok", "tok", "tok"},
				Count:     c,
			})
		}(i)
	}
	wg.Wait()
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// A rotation must actually have happened, or the test isn't exercising the
	// boundary it claims to.
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected a rotated generation %s.1 (no boundary crossed): %v", path, err)
	}

	recovered := readAllGenerations(t, path) // json.Unmarshal here rejects torn lines
	dropped := r.Dropped()
	if len(recovered) == 0 {
		t.Fatal("no decisions recovered from disk: writer never made progress")
	}
	// Conservation bound: nothing is conjured. recovered + dropped <= n, because
	// rotation may discard an older generation but never invents records.
	if int64(len(recovered))+dropped > int64(n) {
		t.Fatalf("conservation violated: recovered=%d + dropped=%d > sent=%d", len(recovered), dropped, n)
	}
	// Every recovered line is a well-formed decision with the expected shape (the
	// padding tokens survived intact — proof the line wasn't torn mid-write).
	for _, d := range recovered {
		if d.Channel != "push" || len(d.Tokens) != 5 {
			t.Fatalf("recovered a malformed/interleaved decision: %+v", d)
		}
	}
}

// TestRecorder_DropsUnderOverloadWithoutBlocking proves the two load-bearing
// properties of the best-effort drop path (decision.go default branch): Record
// never blocks the caller even when the queue cannot keep pace, and overload
// advances Dropped() instead of stalling. A single writer cannot drain a 100k
// tight-loop burst into a Buffer:1 queue, so the default branch fires.
func TestRecorder_DropsUnderOverloadWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	r, err := telemetry.NewRecorder(telemetry.Config{
		Path: path, MaxBytes: 1 << 20, Buffer: 1, Salt: []byte("pepper"), Now: fixedNow,
	})
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	const burst = 100_000
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < burst; i++ {
			r.Record(telemetry.Decision{Channel: "push", QueryHash: "h", Count: i})
		}
	}()

	// If Record ever blocks, the burst never completes and this times out — that is
	// the non-blocking contract under test.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Record blocked under overload: a 100k burst did not complete (queue backpressured the caller)")
	}

	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := r.Dropped(); got == 0 {
		t.Fatalf("expected drops under a 100k tight-loop burst into a depth-1 queue, got Dropped()=%d", got)
	}
}
