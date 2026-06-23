// SPDX-License-Identifier: AGPL-3.0-only

package lock_test

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/lock"
)

// Helper-process env: when set, the test binary runs as a subprocess that takes
// the lock at TWICESHY_LOCK_PATH, signals "LOCKED" on stdout, and blocks so the
// PARENT process observes a *separate process* holding it. Stdlib pattern — no
// new dependency, no flock(1), portable to darwin.
const (
	lockHelperEnv = "TWICESHY_LOCK_HELPER"
	lockPathEnv   = "TWICESHY_LOCK_PATH"
)

func TestMain(m *testing.M) {
	if os.Getenv(lockHelperEnv) == "1" {
		runLockHelper() // never returns
	}
	os.Exit(m.Run())
}

// runLockHelper is the subprocess body: acquire the lock, announce it, then block
// on stdin closing (the parent kills us, or closes the pipe, to release).
func runLockHelper() {
	l, err := lock.Acquire(os.Getenv(lockPathEnv))
	if err != nil {
		fmt.Fprintf(os.Stderr, "helper acquire: %v\n", err)
		os.Exit(2)
	}
	fmt.Println("LOCKED")
	_ = os.Stdout.Sync()
	// Block until stdin reaches EOF (parent closes the pipe / kills us). Holding the
	// lock the whole time is the point: the parent must see ErrHeld meanwhile.
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	_ = l.Release()
	os.Exit(0)
}

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

// The lock exists to stop an overlapping cron tick or a manual run — a SECOND OS
// PROCESS — from double-writing the corpus (lock.go package doc). TestAcquire_
// SecondIsHeld proves only the same-process/different-fd path; this proves the
// real deployment property across a fork/exec, and would fail under a regression
// to per-process fcntl locks (which TestAcquire_SecondIsHeld would still pass).
func TestAcquire_CrossProcessHeld(t *testing.T) {
	if os.Getenv(lockHelperEnv) == "1" {
		// Defensive: in helper mode TestMain already exited; never run the assertions here.
		t.Skip("helper subprocess")
	}
	p := filepath.Join(t.TempDir(), "loop.lock")

	cmd := exec.Command(os.Args[0], "-test.run=TestAcquire_CrossProcessHeld")
	cmd.Env = append(os.Environ(), lockHelperEnv+"=1", lockPathEnv+"="+p)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waited := false
	// Ensure the child is torn down even if an assertion fails mid-test.
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	// Wait for the child to announce it holds the lock (bounded, so a hung child
	// can't wedge the test).
	ready := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		ready <- line
	}()
	select {
	case line := <-ready:
		if line != "LOCKED\n" {
			t.Fatalf("helper did not acquire the lock, got %q", line)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for the helper to acquire the lock")
	}

	// The child (a separate process) holds the lock: the parent must see ErrHeld.
	if _, err := lock.Acquire(p); !errors.Is(err, lock.ErrHeld) {
		t.Fatalf("parent Acquire while child holds the lock = %v, want ErrHeld", err)
	}

	// Release the child (close its stdin → EOF) and wait for it to drop the lock.
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("helper exited with error: %v", err)
	}
	waited = true

	// With the child gone, the parent can now take the lock.
	l, err := lock.Acquire(p)
	if err != nil {
		t.Fatalf("parent Acquire after child released = %v, want success", err)
	}
	_ = l.Release()
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
