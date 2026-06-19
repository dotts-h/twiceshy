// SPDX-License-Identifier: AGPL-3.0-only

// Package lock is the single-flight guard for the mutating loop (ADR-0013 §A2):
// a non-blocking exclusive flock on a corpus-local lockfile, so an overlapping
// cron tick or a manual run during the timer cannot both load the corpus and
// double-write. Unix-only (the deploy + CI target, linux/darwin); the lock is
// released on Release or, as a backstop, when the process exits.
package lock

import (
	"errors"
	"os"
	"syscall"
)

// ErrHeld means another process already holds the lock — the caller should
// report "a run is already in progress" and exit, not block.
var ErrHeld = errors.New("another run holds the lock")

// Lock is a held flock. Call Release to drop it.
type Lock struct{ f *os.File }

// Acquire takes a non-blocking exclusive flock on path (created if absent). It
// returns ErrHeld when another open file description already holds the lock
// (including another goroutine/process), and the underlying error otherwise.
func Acquire(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrHeld
		}
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Release drops the lock and closes the file. It is safe to call on a nil Lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}
