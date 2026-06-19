// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// The Reaper backstop (#0018) must actually be invoked by the loop at startup
// (#0052) — before, it existed but nothing called it, so a crashed run's
// containers/volumes accumulated. startupReap is the wiring seam: these guard
// that it sweeps a real run, stays out of a dry-run, and never aborts the run.

func TestStartupReap_SweepsOnRealRun(t *testing.T) {
	calls := 0
	orig := reapOrphans
	t.Cleanup(func() { reapOrphans = orig })
	reapOrphans = func(context.Context) (int, int, error) { calls++; return 2, 1, nil }

	var buf bytes.Buffer
	startupReap(context.Background(), "promote", false /*dryRun*/, nil, &buf)

	if calls != 1 {
		t.Fatalf("a real run must sweep orphans exactly once; got %d calls", calls)
	}
	if out := buf.String(); !strings.Contains(out, "reaped 2") || !strings.Contains(out, "1 volume") {
		t.Errorf("sweep result should be reported; got %q", out)
	}
}

func TestStartupReap_SkipsDryRun(t *testing.T) {
	calls := 0
	orig := reapOrphans
	t.Cleanup(func() { reapOrphans = orig })
	reapOrphans = func(context.Context) (int, int, error) { calls++; return 0, 0, nil }

	var buf bytes.Buffer
	startupReap(context.Background(), "promote", true /*dryRun=-effect*/, nil, &buf)

	if calls != 0 {
		t.Fatalf("a dry-run (-effect writes nothing) must not delete containers; got %d sweep calls", calls)
	}
	if buf.Len() != 0 {
		t.Errorf("dry-run sweep must be silent; got %q", buf.String())
	}
}

func TestStartupReap_SweepErrorIsNonFatal(t *testing.T) {
	orig := reapOrphans
	t.Cleanup(func() { reapOrphans = orig })
	reapOrphans = func(context.Context) (int, int, error) { return 0, 0, errors.New("docker down") }

	var buf bytes.Buffer
	// Must not panic and must return (no fatal); a best-effort sweep failure is logged.
	startupReap(context.Background(), "adapt", false, nil, &buf)

	if out := buf.String(); !strings.Contains(out, "orphan sweep failed") {
		t.Errorf("a sweep failure should be surfaced, not swallowed; got %q", out)
	}
}
