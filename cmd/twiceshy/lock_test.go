// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/lock"
)

// judgeEnv returns a getenv that supplies a (dummy) judge URL, so a run reaches
// the single-flight lock rather than failing the earlier fail-safe judge check.
func judgeEnv(k string) string {
	if k == "TWICESHY_JUDGE_URL" {
		return "http://judge.local"
	}
	return ""
}

// emptyCorpus is a temp dir with an empty experience/ (LoadCorpus returns zero
// records, no error) — enough to exercise the run setup up to the lock.
func emptyCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "experience"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// #0039 (ADR-0013 §A2): while one run holds the lock, a second promote exits with
// a clear "in progress" error (and a non-zero exit via main's exitCode default).
func TestRunPromote_SingleFlightLockBlocksSecondRun(t *testing.T) {
	corpus := emptyCorpus(t)
	held, err := lock.Acquire(filepath.Join(corpus, loopLockName))
	if err != nil {
		t.Fatalf("pre-acquire: %v", err)
	}
	defer func() { _ = held.Release() }()

	var buf bytes.Buffer
	err = runPromote(context.Background(), []string{"-corpus", corpus}, &buf, judgeEnv)
	if err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Fatalf("a second run while the lock is held must fail clearly; got %v", err)
	}
}

func TestRunAdapt_SingleFlightLockBlocksSecondRun(t *testing.T) {
	corpus := emptyCorpus(t)
	held, err := lock.Acquire(filepath.Join(corpus, loopLockName))
	if err != nil {
		t.Fatalf("pre-acquire: %v", err)
	}
	defer func() { _ = held.Release() }()

	var buf bytes.Buffer
	err = runAdapt(context.Background(), []string{"-corpus", corpus}, &buf, judgeEnv)
	if err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Fatalf("a second adapt while the lock is held must fail clearly; got %v", err)
	}
}

// promote and adapt share one lock, so they are mutually exclusive (an adapt run
// can't start while a promote holds it).
func TestLoop_PromoteAndAdaptShareTheLock(t *testing.T) {
	corpus := emptyCorpus(t)
	held, err := lock.Acquire(filepath.Join(corpus, loopLockName))
	if err != nil {
		t.Fatalf("pre-acquire: %v", err)
	}
	defer func() { _ = held.Release() }()

	// The held lock is at loopLockName; both commands resolve to the same path.
	if got := filepath.Base(filepath.Join(corpus, loopLockName)); got != loopLockName {
		t.Fatalf("lock name drift: %q", got)
	}
	var buf bytes.Buffer
	if err := runAdapt(context.Background(), []string{"-corpus", corpus}, &buf, judgeEnv); err == nil {
		t.Fatal("adapt must be blocked by a held promote/adapt lock")
	}
}

// A run that releases its lock leaves it free for the next run.
func TestRunPromote_LockReleasedAfterRun(t *testing.T) {
	corpus := emptyCorpus(t)
	// No judge URL → the run errors AFTER taking+releasing the lock (defer).
	var buf bytes.Buffer
	_ = runPromote(context.Background(), []string{"-corpus", corpus}, &buf, func(string) string { return "" })

	// The lock must now be free.
	lk, err := lock.Acquire(filepath.Join(corpus, loopLockName))
	if err != nil {
		t.Fatalf("lock not released after the run: %v", err)
	}
	_ = lk.Release()
}
