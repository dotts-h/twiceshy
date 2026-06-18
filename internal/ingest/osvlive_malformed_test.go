// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/ingest"
)

// sourceFromFiles builds an OSVLiveSource over an in-memory zip of the given files.
func sourceFromFiles(t *testing.T, files map[string]any) ingest.Source {
	t.Helper()
	return ingest.NewOSVLiveSource(ingest.WithOSVLiveFetch(func(_ context.Context) (io.ReadCloser, error) {
		return buildOSVLiveZip(t, files), nil
	}))
}

// A malformed advisory — no id, or a Go affected block with no package name —
// must be SKIPPED, not emitted. An emitted id-less draft carries an empty
// error_signature that fails validateSymptom inside Prepare and aborts the whole
// import batch; a bulk importer over an untrusted feed skips junk, never fails
// the batch on one bad entry.
func TestOSVLiveSource_SkipsIDlessAndNamelessEntries(t *testing.T) {
	goAffected := []map[string]any{
		{
			"package": map[string]string{"ecosystem": "Go", "name": "github.com/example/pkg"},
			"ranges":  []map[string]any{{"type": "SEMVER", "events": []map[string]string{{"introduced": "0"}, {"fixed": "1.2.3"}}}},
		},
	}
	files := map[string]any{
		"no-id.json": map[string]any{ // missing "id" — must be skipped, not abort the batch
			"aliases":  []string{},
			"affected": goAffected,
		},
		"nameless.json": map[string]any{ // Go affected with empty package name — skipped
			"id": "GO-2020-NAMELESS",
			"affected": []map[string]any{
				{
					"package": map[string]string{"ecosystem": "Go", "name": ""},
					"ranges":  []map[string]any{{"type": "SEMVER", "events": []map[string]string{{"introduced": "0"}}}},
				},
			},
		},
		"valid.json": map[string]any{
			"id":       "GO-2020-VALID",
			"aliases":  []string{"CVE-2020-1"},
			"affected": goAffected,
		},
	}
	drafts, err := sourceFromFiles(t, files).Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts must skip malformed entries, not error: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("want only the 1 valid draft (id-less + nameless skipped), got %d", len(drafts))
	}
	d := drafts[0]
	if d.Symptom == nil || len(d.Symptom.ErrorSignatures) == 0 || d.Symptom.ErrorSignatures[0] != "GO-2020-VALID" {
		t.Fatalf("surviving draft must be the valid one with a real id, got %+v", d.Symptom)
	}
	for _, sig := range d.Symptom.ErrorSignatures {
		if strings.TrimSpace(sig) == "" {
			t.Errorf("draft carries an empty error_signature (would fail Prepare)")
		}
	}
}

// An advisory with no aliases must not render dangling empty parens in the summary.
func TestOSVLiveSource_EmptyAliasesNoDanglingParens(t *testing.T) {
	files := map[string]any{
		"no-aliases.json": map[string]any{
			"id": "GO-2021-NOALIAS",
			"affected": []map[string]any{
				{
					"package": map[string]string{"ecosystem": "Go", "name": "github.com/example/pkg"},
					"ranges":  []map[string]any{{"type": "SEMVER", "events": []map[string]string{{"fixed": "2.0.0"}}}},
				},
			},
		},
	}
	drafts, err := sourceFromFiles(t, files).Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("want 1 draft, got %d", len(drafts))
	}
	if got := drafts[0].Symptom.Summary; strings.Contains(got, "()") {
		t.Errorf("summary has dangling empty parens: %q", got)
	}
}
