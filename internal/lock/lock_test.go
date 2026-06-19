// SPDX-License-Identifier: AGPL-3.0-only

package lock_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/lock"
)

// The contention path (#0039, ADR-0013 §A2): while one holder has the lock, a
// second Acquire on the same file must fail with ErrHeld, not block or succeed.
func TestAcquire_SecondIsHeld(t *testing.T) {
	p := filepath.Join(t.TempDir(), "loop.lock")
	l1, err := lock.Acquire(p)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer func() { _ = l1.Release() }()

	if _, err := lock.Acquire(p); !errors.Is(err, lock.ErrHeld) {
		t.Fatalf("second acquire = %v, want ErrHeld", err)
	}
}

// After Release the lock is free to take again (the next nightly run).
func TestAcquire_ReleaseAllowsReacquire(t *testing.T) {
	p := filepath.Join(t.TempDir(), "loop.lock")
	l1, err := lock.Acquire(p)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := l1.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	l2, err := lock.Acquire(p)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	_ = l2.Release()
}

// Release on a nil/zero Lock is a no-op (so a `defer lk.Release()` after a failed
// Acquire never panics).
func TestRelease_NilSafe(t *testing.T) {
	var l *lock.Lock
	if err := l.Release(); err != nil {
		t.Fatalf("nil release: %v", err)
	}
}
