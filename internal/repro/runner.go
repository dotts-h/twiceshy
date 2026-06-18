// SPDX-License-Identifier: AGPL-3.0-only

package repro

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// maxCapture bounds how much stdout/stderr the runner keeps per stream. The
// container's memory cap does NOT limit host-side buffering, so without this an
// untrusted script could exhaust the broker's memory by spewing output. Excess
// is dropped and the stream is marked truncated.
const maxCapture = 1 << 20 // 1 MiB

// capWriter keeps at most n bytes; further writes are counted but discarded.
type capWriter struct {
	buf       bytes.Buffer
	remaining int
	truncated bool
}

func newCapWriter(n int) *capWriter { return &capWriter{remaining: n} }

func (w *capWriter) Write(p []byte) (int, error) {
	if w.remaining > 0 {
		take := len(p)
		if take > w.remaining {
			take = w.remaining
		}
		w.buf.Write(p[:take])
		w.remaining -= take
	}
	if len(p) > 0 && w.remaining == 0 {
		w.truncated = true
	}
	return len(p), nil // always report full consumption so the child never blocks
}

func (w *capWriter) String() string {
	if w.truncated {
		return w.buf.String() + "\n[...output truncated by twiceshy broker...]"
	}
	return w.buf.String()
}

// execResult is the outcome of one external command.
type execResult struct {
	stdout   string
	stderr   string
	exitCode int
	timedOut bool
}

// commandRunner runs an external command with an optional stdin payload and a
// hard wall-clock timeout. It is the broker's single seam onto the host: unit
// tests inject a stub so no Docker daemon is required.
type commandRunner interface {
	run(ctx context.Context, stdin []byte, timeout time.Duration, name string, args ...string) (execResult, error)
}

// dockerRunner is the production commandRunner: it shells out to the docker CLI.
// Using the CLI (not a Go SDK) keeps twiceshy inside its dependency budget.
type dockerRunner struct{}

func (dockerRunner) run(ctx context.Context, stdin []byte, timeout time.Duration, name string, args ...string) (execResult, error) {
	if timeout <= 0 {
		timeout = DefaultLimits.Timeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, errb := newCapWriter(maxCapture), newCapWriter(maxCapture)
	cmd.Stdout = out
	cmd.Stderr = errb

	err := cmd.Run()
	res := execResult{stdout: out.String(), stderr: errb.String()}
	if cctx.Err() == context.DeadlineExceeded {
		res.timedOut = true
		res.exitCode = -1
		return res, cctx.Err()
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.exitCode = ee.ExitCode()
			return res, nil // a non-zero exit is a valid result, not a runner error
		}
		return res, err // the command could not be started/found
	}
	return res, nil
}
