// SPDX-License-Identifier: AGPL-3.0-only

package repro

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Healthy is the preflight probe (#0040, ADR-0013 §A3): docker reachable AND the
// runsc runtime registered. It must distinguish a dead daemon from a missing
// runtime so the abort names the real cause.
func TestBrokerHealthy_OKWhenRuntimePresent(t *testing.T) {
	s := &stubRunner{responder: func(rc recordedCall) (execResult, error) {
		return execResult{stdout: "runc\nrunsc\n"}, nil
	}}
	b := NewBroker([]string{"golang:1.25"}, withRunner(s), WithRuntime("runsc"))
	if err := b.Healthy(context.Background()); err != nil {
		t.Fatalf("healthy substrate must pass: %v", err)
	}
}

func TestBrokerHealthy_DaemonDown(t *testing.T) {
	s := &stubRunner{responder: func(rc recordedCall) (execResult, error) {
		return execResult{exitCode: 1, stderr: "Cannot connect to the Docker daemon"}, errors.New("exit status 1")
	}}
	b := NewBroker([]string{"golang:1.25"}, withRunner(s), WithRuntime("runsc"))
	err := b.Healthy(context.Background())
	if err == nil || !strings.Contains(err.Error(), "docker daemon not reachable") {
		t.Fatalf("a dead daemon must be named; got %v", err)
	}
}

func TestBrokerHealthy_RuntimeMissing(t *testing.T) {
	s := &stubRunner{responder: func(rc recordedCall) (execResult, error) {
		return execResult{stdout: "runc\nio.containerd.runc.v2\n"}, nil // no runsc
	}}
	b := NewBroker([]string{"golang:1.25"}, withRunner(s), WithRuntime("runsc"))
	err := b.Healthy(context.Background())
	if err == nil || !strings.Contains(err.Error(), "runsc") {
		t.Fatalf("a missing runsc runtime must be named; got %v", err)
	}
}
