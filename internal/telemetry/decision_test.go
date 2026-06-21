// SPDX-License-Identifier: AGPL-3.0-only

package telemetry_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
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
