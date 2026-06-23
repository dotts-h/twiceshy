// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/testcorpus"
)

const usageTestRepo = "github.com/dotts-h/twiceshy"

func fixedClock() time.Time { return time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC) }

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func usageFixture() *record.Record {
	return &record.Record{
		SchemaVersion: 1, ID: "exp-0200", Kind: "trap", Status: "validated",
		Title: "usage wiring fixture record with a sufficiently long title",
		Symptom: &record.Symptom{
			Summary:         "a distinctive symptom for the usage wiring test",
			ErrorSignatures: []string{"zzdistinct-usage-signature-marker"},
		},
		Path: "experience/2026/0200-usage-fixture.md",
		Provenance: record.Provenance{
			Source: record.Source{Author: "test"}, RecordedAt: "2026-06-19",
			ValidatedAt: ptr("2026-06-19"), Valid: record.Validity{From: "2026-06-19"},
		},
	}
}

func ptr(s string) *string { return &s }

func newUsageHandlers(t *testing.T, recs ...*record.Record) (*handlers, *index.Index) {
	t.Helper()
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, usageTestRepo); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	logger := quietLogger()
	h := &handlers{ix: ix, repo: usageTestRepo, logger: logger}
	h.usage = newUsageRecorder(ix, logger, fixedClock)
	return h, ix
}

func TestSearchRecordsUsage(t *testing.T) {
	h, ix := newUsageHandlers(t, usageFixture())
	_, out, err := h.search(context.Background(), nil, SearchArgs{Query: "zzdistinct-usage-signature-marker"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(out.Hits) == 0 || out.Hits[0].ID != "exp-0200" {
		t.Fatalf("expected the fixture as a hit, got %+v", out.Hits)
	}
	h.usage.flush()

	u, err := ix.Usage(context.Background(), "exp-0200")
	if err != nil {
		t.Fatal(err)
	}
	if u.Retrieved != 1 {
		t.Fatalf("retrieved = %d, want 1 (a served record's usage must advance)", u.Retrieved)
	}
	if u.LastHit == nil || *u.LastHit != "2026-06-19" {
		t.Fatalf("last_hit = %v, want 2026-06-19", u.LastHit)
	}
}

func TestGetRecordsUsage(t *testing.T) {
	h, ix := newUsageHandlers(t, usageFixture())
	if _, _, err := h.get(context.Background(), nil, GetArgs{ID: "exp-0200"}); err != nil {
		t.Fatalf("get: %v", err)
	}
	h.usage.flush()
	u, _ := ix.Usage(context.Background(), "exp-0200")
	if u.Retrieved != 1 {
		t.Fatalf("get retrieved = %d, want 1", u.Retrieved)
	}
}

func TestSearchEmptyResultRecordsNoUsage(t *testing.T) {
	h, ix := newUsageHandlers(t, usageFixture())
	_, out, err := h.search(context.Background(), nil, SearchArgs{Query: "nothing-here-matches-anything-zzz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Hits) != 0 {
		t.Fatalf("expected no hits, got %+v", out.Hits)
	}
	h.usage.flush()
	u, _ := ix.Usage(context.Background(), "exp-0200")
	if u.Retrieved != 0 {
		t.Fatalf("a miss must not bump usage; retrieved = %d", u.Retrieved)
	}
}

// TestPushRecordsImpression closes the push feedback loop end-to-end: a push that
// injects a card bumps the `pushed` impression counter for that record (off the
// latency budget), distinct from the pull `retrieved` counter, and a push that
// injects nothing records no impression.
func TestPushRecordsImpression(t *testing.T) {
	ctx := context.Background()
	recs, err := record.LoadCorpus(testcorpus.Root())
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	ix, err := index.Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(ctx, recs, usageTestRepo); err != nil {
		t.Fatal(err)
	}
	logger := quietLogger()
	h := &handlers{ix: ix, repo: usageTestRepo, logger: logger}
	h.usage = newUsageRecorder(ix, logger, fixedClock)

	push := func(query string) PushResult {
		body := strings.NewReader(`{"query":` + jsonString(query) + `}`)
		req := httptest.NewRequest(http.MethodPost, "/push", body)
		rec := httptest.NewRecorder()
		h.pushHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("push %q: status %d, body %s", query, rec.Code, rec.Body.String())
		}
		var out PushResult
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode push result: %v", err)
		}
		return out
	}

	// A strong, on-domain query injects exp-0001 (the FTS5 trap).
	out := push(`FTS5: syntax error near "."`)
	hasID := func(ids []string, want string) bool {
		for _, id := range ids {
			if id == want {
				return true
			}
		}
		return false
	}
	if !hasID(out.IDs, "exp-0001") {
		t.Fatalf("expected exp-0001 injected, got ids=%v count=%d", out.IDs, out.Count)
	}
	h.usage.flush()

	u, err := ix.Usage(ctx, "exp-0001")
	if err != nil {
		t.Fatal(err)
	}
	if u.Pushed != 1 {
		t.Fatalf("pushed = %d, want 1 (an injected card's impression must be recorded)", u.Pushed)
	}
	if u.Retrieved != 0 {
		t.Fatalf("retrieved = %d, want 0 (a push impression is not a pull)", u.Retrieved)
	}

	// An off-domain query injects nothing and records no impression.
	before, _ := ix.Usage(ctx, "exp-0001")
	out = push("what is a good birthday gift to buy for my mother")
	if out.Count != 0 || len(out.IDs) != 0 {
		t.Fatalf("off-domain push must inject nothing, got count=%d ids=%v", out.Count, out.IDs)
	}
	h.usage.flush()
	after, _ := ix.Usage(ctx, "exp-0001")
	if after.Pushed != before.Pushed {
		t.Fatalf("a push that injects nothing must not record an impression: %d -> %d", before.Pushed, after.Pushed)
	}
}

// jsonString quotes s as a JSON string literal for the test request body.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// fakeUsageStore records calls and can be primed to fail.
type fakeUsageStore struct {
	mu        sync.Mutex
	calls     [][]string
	dates     []string
	pushCalls [][]string
	err       error
}

func (f *fakeUsageStore) RecordHits(_ context.Context, ids []string, date string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, ids)
	f.dates = append(f.dates, date)
	return f.err
}

func (f *fakeUsageStore) RecordPushes(_ context.Context, ids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pushCalls = append(f.pushCalls, ids)
	return f.err
}

func TestUsageRecorderRecordPushAsync(t *testing.T) {
	f := &fakeUsageStore{}
	r := newUsageRecorder(f, quietLogger(), fixedClock)
	r.recordPush([]string{"exp-1", "exp-2"})
	r.recordPush(nil)        // no-op
	r.recordPush([]string{}) // no-op
	r.flush()

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pushCalls) != 1 {
		t.Fatalf("got %d push writes, want 1 (empty lists are no-ops)", len(f.pushCalls))
	}
	if got := f.pushCalls[0]; len(got) != 2 || got[0] != "exp-1" {
		t.Fatalf("recorded push ids = %v, want [exp-1 exp-2]", got)
	}
	if len(f.calls) != 0 {
		t.Fatalf("recordPush must not touch the retrieved counter; got %d hit writes", len(f.calls))
	}
}

func TestUsageRecorderRecordPushNilSafe(t *testing.T) {
	var r *usageRecorder
	r.recordPush([]string{"exp-1"}) // nil recorder is a no-op, not a panic
	r.flush()
}

func TestUsageRecorderAsyncAndDate(t *testing.T) {
	f := &fakeUsageStore{}
	r := newUsageRecorder(f, quietLogger(), fixedClock)
	r.record([]string{"exp-1", "exp-2"})
	r.record(nil)        // no-op
	r.record([]string{}) // no-op
	r.flush()

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 1 {
		t.Fatalf("got %d writes, want 1 (empty lists are no-ops)", len(f.calls))
	}
	if got := f.calls[0]; len(got) != 2 || got[0] != "exp-1" {
		t.Fatalf("recorded ids = %v, want [exp-1 exp-2]", got)
	}
	if f.dates[0] != "2026-06-19" {
		t.Fatalf("date = %q, want 2026-06-19 (UTC)", f.dates[0])
	}
}

func TestUsageRecorderSwallowsErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	f := &fakeUsageStore{err: errors.New("db is closed")}
	r := newUsageRecorder(f, logger, fixedClock)
	r.record([]string{"exp-1"}) // a failing usage write must not block or crash
	r.flush()

	// The error path must actually have been REACHED (the write attempted), not
	// silently skipped: assert the store was called with the served id.
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 1 {
		t.Fatalf("store was not called: got %d hit writes, want 1 (the failing write must still be attempted)", len(f.calls))
	}
	if got := f.calls[0]; len(got) != 1 || got[0] != "exp-1" {
		t.Fatalf("recorded ids = %v, want [exp-1]", got)
	}
	// The failure must be logged at Warn, never returned — the package contract.
	if !strings.Contains(buf.String(), "usage record failed") {
		t.Fatalf("a failed usage write must be logged at Warn; logs:\n%s", buf.String())
	}
}

func TestUsageRecorderRecordPushSwallowsErrors(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	f := &fakeUsageStore{err: errors.New("db is closed")}
	r := newUsageRecorder(f, logger, fixedClock)
	r.recordPush([]string{"exp-1"}) // a failing push-impression write must not block or crash
	r.flush()

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pushCalls) != 1 {
		t.Fatalf("store was not called: got %d push writes, want 1 (the failing write must still be attempted)", len(f.pushCalls))
	}
	if got := f.pushCalls[0]; len(got) != 1 || got[0] != "exp-1" {
		t.Fatalf("recorded push ids = %v, want [exp-1]", got)
	}
	if !strings.Contains(buf.String(), "usage recordPush failed") {
		t.Fatalf("a failed push-impression write must be logged at Warn; logs:\n%s", buf.String())
	}
}

// panicUsageStore panics on every write, to exercise the recover() guards that
// keep a panicking usage write from ever crashing a retrieval (usage.go).
type panicUsageStore struct{}

func (panicUsageStore) RecordHits(context.Context, []string, string) error { panic("boom") }
func (panicUsageStore) RecordPushes(context.Context, []string) error       { panic("boom") }

// TestUsageRecorderRecoversFromPanic locks the documented "a panicking usage
// write never crashes a retrieval" contract for BOTH async write paths. If a
// recover() guard were removed, the goroutine would never call wg.Done() and
// flush() would deadlock (the -race / test timeout fails the test), and the
// expected marker would be absent.
func TestUsageRecorderRecoversFromPanic(t *testing.T) {
	for _, tc := range []struct {
		name   string
		call   func(r *usageRecorder)
		marker string
	}{
		{"record", func(r *usageRecorder) { r.record([]string{"exp-1"}) }, "usage record panicked"},
		{"recordPush", func(r *usageRecorder) { r.recordPush([]string{"exp-1"}) }, "usage recordPush panicked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, nil))
			r := newUsageRecorder(panicUsageStore{}, logger, fixedClock)
			tc.call(r)
			r.flush() // returns only if the goroutine recovered and called wg.Done()
			if !strings.Contains(buf.String(), tc.marker) {
				t.Fatalf("expected %q in logs after a panicking write; logs:\n%s", tc.marker, buf.String())
			}
		})
	}
}

func TestUsageRecorderNilSafe(t *testing.T) {
	var r *usageRecorder
	r.record([]string{"exp-1"}) // nil recorder is a no-op, not a panic
	r.flush()
}

func TestUsageRecorderOwnsSlice(t *testing.T) {
	f := &fakeUsageStore{}
	r := newUsageRecorder(f, quietLogger(), fixedClock)
	ids := []string{"exp-1"}
	r.record(ids)
	ids[0] = "mutated" // caller reuses the slice after record returns
	r.flush()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls[0][0] != "exp-1" {
		t.Fatalf("recorder did not copy the slice; saw %q", f.calls[0][0])
	}
}
