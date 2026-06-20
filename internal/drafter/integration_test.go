// SPDX-License-Identifier: AGPL-3.0-only

package drafter_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/drafter"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/repro"
)

// Integration tests need a Docker daemon with the runsc runtime — which the
// socketless CI runner (ADR-0012) deliberately lacks. They run only when
// TWICESHY_REPRO_INTEGRATION=1 (set on the brain, which has docker+runsc).
func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("TWICESHY_REPRO_INTEGRATION") != "1" {
		t.Skip("set TWICESHY_REPRO_INTEGRATION=1 to run real-runsc integration tests")
	}
}

// TestIntegration_DraftedDeprecationReproHoldsUnderGate is the slice-2 proof: a
// repro the deterministic drafter GENERATED (not hand-written) installs staticcheck
// in the prepare phase and proves the deprecation offline — and the pipeline
// attaches it into the record's guard, still quarantined. Every stdlib-only
// catalog entry is proven end-to-end (a drift in staticcheck's verdict for any of
// them would fail here, not silently in production).
func TestIntegration_DraftedDeprecationReproHoldsUnderGate(t *testing.T) {
	requireIntegration(t)
	cases := []struct {
		id, pkg, diagnostic string
	}{
		{"exp-9100", "io/ioutil", "SA1019: ioutil.ReadFile is deprecated: As of Go 1.16, this function simply calls os.ReadFile."},
		{"exp-9101", "math/rand", "SA1019: rand.Seed is deprecated: As of Go 1.20 there is no reason to call Seed with a random value."},
		// Third-party fix class: the prepare phase warms golang.org/x/text (networked)
		// so staticcheck type-checks the cases.Title fix offline in execute (#0026).
		{"exp-9102", "strings", "SA1019: strings.Title is deprecated: The rule Title uses for word boundaries does not handle Unicode punctuation properly."},
	}
	for _, tc := range cases {
		t.Run(tc.pkg, func(t *testing.T) {
			root := t.TempDir()
			b := repro.NewBroker([]string{repro.PinnedGoImage},
				repro.WithLimits(repro.Limits{
					Memory: "1g", CPUs: "2.0", PidsLimit: 256, TmpfsSize: "128m",
					Timeout: 5 * time.Minute,
				}))
			rv := repro.NewRevalidator(b, root)
			p := drafter.NewPipeline(rv, root, drafter.NewGoDeprecationDrafter())

			rec := &record.Record{
				ID:        tc.id,
				Status:    "quarantined",
				Path:      "experience/2026/" + tc.id + ".md",
				Symptom:   &record.Symptom{ErrorSignatures: []string{tc.diagnostic}},
				AppliesTo: []record.AppliesTo{{Ecosystem: "Go", Package: tc.pkg}},
			}

			out, err := p.Run(context.Background(), rec)
			if err != nil {
				t.Fatalf("pipeline Run: %v", err)
			}
			if !out.Attached {
				t.Fatalf("drafted deprecation repro should hold via prepare+execute; outcome=%+v\natt=%+v", out, out.Attestation)
			}
			if rec.Guard == nil || len(rec.Guard.Repros) != 1 || rec.Guard.Repros[0].Kind != "positive" {
				t.Fatalf("proven repro not attached to guard: %+v", rec.Guard)
			}
			if rec.Status != "quarantined" {
				t.Errorf("attach must not promote past quarantined; status=%q", rec.Status)
			}
		})
	}
}
