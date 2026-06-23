// SPDX-License-Identifier: AGPL-3.0-only

// Package telemetry records per-query gate decisions for the retrieval channels
// (#0067): for each /push and search_experience query it appends a structured
// JSON line capturing why a card was or wasn't served, so precision/recall can be
// measured on real traffic and the retro analyzer (#0065) can compute a real
// helpfulness signal. It is write-only and structurally separate from the index
// (it CANNOT influence ranking — ADR-0013 §4), off the hot path (a single async
// writer; Record never blocks serving and drops under overload), privacy-preserving
// (the raw query is hashed, never persisted), and bounded (size-rotated, so the
// log can never grow without limit).
package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// ServedHit is one injected record id and the score it was served at.
type ServedHit struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

// Decision is the structured record of one retrieval query's gate decision. It
// never carries the raw query — only its salted hash and the retrieval tokens
// (already content-filtered: stopwords and ecosystem names excluded).
type Decision struct {
	Time              string      `json:"ts"`                  // RFC3339; stamped by the recorder
	Channel           string      `json:"channel"`             // "push" | "search"
	QueryHash         string      `json:"query_hash"`          // salted hash, for correlation without the text
	Tokens            []string    `json:"tokens,omitempty"`    // retrieval tokens used (push: discriminative; search: fts)
	FingerprintBypass bool        `json:"fp_bypass,omitempty"` // push: a deterministic stack match bypassed the gate
	Served            []ServedHit `json:"served,omitempty"`
	Count             int         `json:"count"`
}

// Config configures a Recorder.
type Config struct {
	Path     string           // append-only JSONL log path (required)
	MaxBytes int64            // rotate when the active file would exceed this (0 = never)
	Buffer   int              // queue depth before Record drops (0 = a sane default)
	Salt     []byte           // per-deployment salt for query hashing
	Log      *slog.Logger     // nil = JSON on stderr
	Now      func() time.Time // nil = time.Now (injectable for tests)
}

// Recorder appends decisions to a rotating JSONL log via a single writer
// goroutine. A nil *Recorder is a valid no-op (telemetry disabled).
type Recorder struct {
	maxBytes int64
	salt     []byte
	log      *slog.Logger
	now      func() time.Time

	path string
	f    *os.File // owned solely by the writer goroutine — no lock needed
	size int64

	ch        chan Decision
	done      chan struct{}
	closeOnce sync.Once
	dropped   atomic.Int64
}

// NewRecorder opens (creating) the JSONL log and starts the writer goroutine.
func NewRecorder(cfg Config) (*Recorder, error) {
	if cfg.Path == "" {
		return nil, errors.New("telemetry: a log path is required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	buf := cfg.Buffer
	if buf <= 0 {
		buf = 256
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return nil, fmt.Errorf("telemetry: mkdir: %w", err)
	}
	f, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("telemetry: open %s: %w", cfg.Path, err)
	}
	var size int64
	if st, err := f.Stat(); err == nil {
		size = st.Size()
	}
	r := &Recorder{
		maxBytes: cfg.MaxBytes, salt: cfg.Salt, log: log, now: now,
		path: cfg.Path, f: f, size: size,
		ch: make(chan Decision, buf), done: make(chan struct{}),
	}
	go r.run()
	return r, nil
}

// Hash returns the salted hash of a query, for correlation without persisting the
// text. A nil Recorder returns "".
func (r *Recorder) Hash(query string) string {
	if r == nil {
		return ""
	}
	h := sha256.New()
	h.Write(r.salt)
	h.Write([]byte(query))
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// Record enqueues one decision for the async writer. Non-blocking and nil-safe:
// if the queue is full it drops the decision (best-effort — telemetry must never
// slow or block serving) and bumps a counter. The recorder stamps the time.
func (r *Recorder) Record(d Decision) {
	if r == nil {
		return
	}
	if d.Time == "" {
		d.Time = r.now().UTC().Format(time.RFC3339)
	}
	select {
	case r.ch <- d:
	default:
		if n := r.dropped.Add(1); n == 1 || n%1000 == 0 {
			r.log.Warn("telemetry queue full — dropping decisions", slog.Int64("dropped", n))
		}
	}
}

// run is the single writer goroutine: it owns the file, so append + rotate need
// no lock, and FIFO order is preserved. It keeps draining the channel even after a
// fatal file error (r.f == nil) so Record never blocks. Exits on channel close.
func (r *Recorder) run() {
	defer close(r.done)
	defer func() {
		if r.f != nil {
			_ = r.f.Close()
		}
	}()
	for d := range r.ch {
		if r.f == nil {
			continue // a rotation failed earlier; keep draining, write nothing
		}
		line, err := json.Marshal(d)
		if err != nil {
			r.log.Warn("telemetry marshal failed", slog.String("error", err.Error()))
			continue
		}
		line = append(line, '\n')
		if r.maxBytes > 0 && r.size > 0 && r.size+int64(len(line)) > r.maxBytes {
			r.rotate()
			if r.f == nil {
				continue
			}
		}
		n, err := r.f.Write(line)
		if err != nil {
			r.log.Warn("telemetry write failed", slog.String("error", err.Error()))
			continue
		}
		r.size += int64(n)
	}
}

// rotate renames the active log to "<path>.1" (keeping exactly one prior
// generation, so on-disk is bounded to ~2*MaxBytes) and opens a fresh active
// file. Writer-goroutine only. On any failure it sets r.f = nil, so run() stops
// writing but keeps draining — telemetry degrades, serving is untouched.
func (r *Recorder) rotate() {
	if err := r.f.Close(); err != nil {
		r.log.Warn("telemetry rotate close failed — telemetry stops", slog.String("error", err.Error()))
		r.f = nil
		return
	}
	if err := os.Rename(r.path, r.path+".1"); err != nil {
		// The active file is already closed; nulling r.f stops telemetry rather than
		// reopening and appending to the un-rotated file (which would blow the
		// ~2*MaxBytes on-disk bound). run() keeps draining; serving is untouched.
		r.log.Warn("telemetry rotate rename failed — telemetry stops", slog.String("error", err.Error()))
		r.f = nil
		return
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		r.log.Error("telemetry rotate reopen failed — telemetry stops", slog.String("error", err.Error()))
		r.f = nil
		return
	}
	r.f = f
	r.size = 0
}

// Close stops the writer, draining queued decisions and closing the file. Nil-safe
// and idempotent.
func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() { close(r.ch) })
	<-r.done
	return nil
}

// Dropped reports how many decisions were dropped because the queue was full.
func (r *Recorder) Dropped() int64 {
	if r == nil {
		return 0
	}
	return r.dropped.Load()
}
