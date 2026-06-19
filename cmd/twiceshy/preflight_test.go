// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeHealth struct{ err error }

func (f fakeHealth) Healthy(context.Context) error { return f.err }

type fakePing struct{ err error }

func (f fakePing) Ping(context.Context) error { return f.err }

// Preflight aborts up front and names which check failed (#0040, ADR-0013 §A3).
func TestPreflight_BrokerDownNamesSubstrate(t *testing.T) {
	err := preflight(context.Background(), fakeHealth{err: errors.New("docker daemon not reachable")}, fakePing{})
	if !errors.Is(err, errPreflight) {
		t.Fatalf("a broker-down preflight must wrap errPreflight; got %v", err)
	}
	if !strings.Contains(err.Error(), "broker") {
		t.Fatalf("the abort must name the broker substrate; got %v", err)
	}
}

func TestPreflight_JudgeDownNamesJudge(t *testing.T) {
	err := preflight(context.Background(), fakeHealth{}, fakePing{err: errors.New("endpoint unreachable")})
	if !errors.Is(err, errPreflight) {
		t.Fatalf("a judge-down preflight must wrap errPreflight; got %v", err)
	}
	if !strings.Contains(err.Error(), "judge") {
		t.Fatalf("the abort must name the judge liveness check; got %v", err)
	}
}

func TestPreflight_AllHealthy(t *testing.T) {
	if err := preflight(context.Background(), fakeHealth{}, fakePing{}); err != nil {
		t.Fatalf("a healthy substrate must pass preflight: %v", err)
	}
}

// The broker check runs first, so a both-down substrate is reported as broker.
func TestPreflight_BrokerCheckedFirst(t *testing.T) {
	err := preflight(context.Background(), fakeHealth{err: errors.New("down")}, fakePing{err: errors.New("also down")})
	if !strings.Contains(err.Error(), "broker") {
		t.Fatalf("broker is probed first; got %v", err)
	}
}

func TestExitCode_PreflightIsDistinct(t *testing.T) {
	if got := exitCode(errors.Join(errors.New("ctx"), errPreflight)); got != 4 {
		t.Fatalf("preflight failure must map to exit 4, got %d", got)
	}
}
