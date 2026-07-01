// SPDX-License-Identifier: AGPL-3.0-only

package agenteval

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/repro"
)

// TestLive_AgentEval drives the slice-2 eval against a REAL off-pool model and the
// gVisor broker — the live numbers. It is skipped unless TWICESHY_AGENTEVAL_LIVE=1, so
// CI (which never sets it, and has no model endpoint) stays deterministic, mirroring
// internal/repro's TWICESHY_REPRO_INTEGRATION gate. Configure:
//
//	TWICESHY_AGENTEVAL_URL    base, no /v1 suffix (e.g. https://integrate.api.nvidia.com)
//	TWICESHY_AGENTEVAL_MODEL  e.g. qwen/qwen3.5-397b-a17b
//	TWICESHY_AGENTEVAL_KEY    bearer token (falls back to NVIDIA_API_KEY)
//
// It runs only react19-useref — the cleanly-executable trap (a tsc type error) — and
// logs each arm's model output + token cost + avoidance verdict, so the signal can be
// eyeballed, not just aggregated.
func TestLive_AgentEval(t *testing.T) {
	if os.Getenv("TWICESHY_AGENTEVAL_LIVE") != "1" {
		t.Skip("set TWICESHY_AGENTEVAL_LIVE=1 (+ endpoint/model/key) to run the live agent eval")
	}
	key := os.Getenv("TWICESHY_AGENTEVAL_KEY")
	if key == "" {
		key = os.Getenv("NVIDIA_API_KEY")
	}
	runner, err := NewModelRunner(RunnerConfig{
		Endpoint: os.Getenv("TWICESHY_AGENTEVAL_URL"),
		Model:    os.Getenv("TWICESHY_AGENTEVAL_MODEL"),
		APIKey:   key,
	})
	if err != nil {
		t.Fatalf("NewModelRunner: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()

	broker := repro.NewBroker([]string{pinnedNodeImage, repro.PinnedGoImage})
	if err := broker.Healthy(ctx); err != nil {
		t.Skipf("broker substrate not healthy (need docker + runsc): %v", err)
	}
	verifier := NewBrokerVerifier(broker)

	var tc TaskCase
	for _, c := range allProspectAndGoldTasks() {
		if c.VerifyID == "react19-useref" {
			tc = c
		}
	}
	if tc.VerifyID == "" {
		t.Fatal("react19-useref gold task not found")
	}

	for _, arm := range []struct {
		name string
		card string
	}{{"memory-OFF", ""}, {"memory-ON", tc.Card}} {
		res, err := runner.Run(ctx, tc.Prompt, arm.card)
		if err != nil {
			t.Fatalf("%s Run: %v", arm.name, err)
		}
		avoided, err := verifier.Avoided(ctx, tc, res.Output)
		if err != nil {
			t.Fatalf("%s Avoided: %v", arm.name, err)
		}
		t.Logf("\n=== %s ===\ntokens=%d  AVOIDED=%v\n--- model output ---\n%s\n", arm.name, res.Tokens, avoided, res.Output)
	}
}

// TestLive_VerifierDiscriminates is the instrument check: an avoidance verifier is only
// trustworthy if it FAILS a trap-hitting input and PASSES a clean one. Without this, a
// broken verifier (e.g. @types/react not installed) would silently score everything
// "avoided" and the eval would be vacuous. No model — just the broker.
func TestLive_VerifierDiscriminates(t *testing.T) {
	if os.Getenv("TWICESHY_AGENTEVAL_LIVE") != "1" {
		t.Skip("set TWICESHY_AGENTEVAL_LIVE=1 to run the live verifier discrimination check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	broker := repro.NewBroker([]string{pinnedNodeImage, repro.PinnedGoImage})
	if err := broker.Healthy(ctx); err != nil {
		t.Skipf("broker substrate not healthy: %v", err)
	}
	v := NewBrokerVerifier(broker)
	c := TaskCase{VerifyID: "react19-useref"}

	// Negative control: the trap itself — the zero-argument useRef overload @types/react@19
	// dropped → TS2554. A correct verifier must report AVOIDED=false.
	hit := "import { useRef } from 'react';\nexport const r = useRef<number>();\n"
	if avoided, err := v.Avoided(ctx, c, hit); err != nil {
		t.Fatalf("hit-control Avoided: %v", err)
	} else if avoided {
		t.Error("INSTRUMENT BROKEN: zero-arg useRef<number>() must be AVOIDED=false (TS2554) — the verifier is not discriminating")
	} else {
		t.Log("negative control OK: zero-arg useRef → AVOIDED=false (trap caught)")
	}

	// Positive control: an explicit initial value type-checks clean.
	clean := "import { useRef } from 'react';\nexport const r = useRef<number | null>(null);\n"
	if avoided, err := v.Avoided(ctx, c, clean); err != nil {
		t.Fatalf("clean-control Avoided: %v", err)
	} else if !avoided {
		t.Error("false negative: explicit-init useRef must be AVOIDED=true")
	} else {
		t.Log("positive control OK: useRef(null) → AVOIDED=true")
	}
}
