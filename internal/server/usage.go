// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// usageWriteTimeout bounds a single async usage write so a stalled DB can't pin
// a goroutine forever.
const usageWriteTimeout = 5 * time.Second

// usageStore is the slice of the index the read path needs to record usage:
// only an increment, never a read. Narrowing it to an interface keeps the
// recorder unit-testable with a fake and documents that retrieval never lets
// usage influence which records are returned (CONVENTIONS, memory-poisoning).
type usageStore interface {
	RecordHits(ctx context.Context, ids []string, date string) error
}

// usageRecorder writes retrieval usage off the request's latency budget
// (ADR-0013 §4): the search/get handler hands it the served ids and returns
// immediately; the write happens in a short tracked goroutine, never a hot-path
// synchronous write. A failing or panicking usage write never blocks or crashes
// a retrieval. flush() drains in-flight writes for tests and graceful shutdown.
type usageRecorder struct {
	store usageStore
	log   *slog.Logger
	now   func() time.Time
	wg    sync.WaitGroup
}

func newUsageRecorder(store usageStore, log *slog.Logger, now func() time.Time) *usageRecorder {
	if now == nil {
		now = time.Now
	}
	return &usageRecorder{store: store, log: log, now: now}
}

// record schedules an async usage bump for the served ids. Non-blocking; an
// empty list (or a nil recorder) is a no-op.
func (u *usageRecorder) record(ids []string) {
	if u == nil || len(ids) == 0 {
		return
	}
	date := u.now().UTC().Format("2006-01-02")
	owned := append([]string(nil), ids...) // own the slice; the caller may reuse it
	u.wg.Add(1)
	go func() {
		defer u.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				u.log.Error("usage record panicked", slog.Any("panic", r))
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), usageWriteTimeout)
		defer cancel()
		if err := u.store.RecordHits(ctx, owned, date); err != nil {
			u.log.Warn("usage record failed", slog.Int("ids", len(owned)), slog.String("error", err.Error()))
		}
	}()
}

// flush waits for in-flight usage writes to finish. Tests call it for
// determinism; a graceful shutdown would too.
func (u *usageRecorder) flush() {
	if u != nil {
		u.wg.Wait()
	}
}
