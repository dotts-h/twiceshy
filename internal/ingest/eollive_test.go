// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

// cancelDuringReadReader cancels the run on first Read and returns an error, so the
// JSON decode fails with the context already cancelled (a cancel landing mid-decode).
type cancelDuringReadReader struct{ cancel context.CancelFunc }

func (r *cancelDuringReadReader) Read(_ []byte) (int, error) {
	r.cancel()
	return 0, errors.New("read aborted")
}

// A run cancelled while a product body is being decoded must fail loud (the cancel
// is systemic), not be swallowed as a per-product malformed-body skip.
func TestEOLLive_DraftsPropagatesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fetch := func(_ context.Context, _ string) (io.ReadCloser, error) {
		return io.NopCloser(&cancelDuringReadReader{cancel: cancel}), nil
	}
	src := ingest.NewEOLLiveSource(
		ingest.WithEOLProducts([]string{"python"}),
		ingest.WithEOLLiveFetch(fetch),
	)
	if _, err := src.Drafts(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// eolFixtureFetch injects per-product endoflife.date JSON bodies (zero network); an
// unknown product returns a nil reader, mirroring the production 404→skip.
func eolFixtureFetch(bodies map[string]string) func(context.Context, string) (io.ReadCloser, error) {
	return func(_ context.Context, product string) (io.ReadCloser, error) {
		body, ok := bodies[product]
		if !ok {
			return nil, nil
		}
		return io.NopCloser(strings.NewReader(body)), nil
	}
}

func eolSigs(drafts []ingest.Draft) []string {
	var out []string
	for _, d := range drafts {
		if d.Symptom != nil && len(d.Symptom.ErrorSignatures) > 0 {
			out = append(out, d.Symptom.ErrorSignatures[0])
		}
	}
	return out
}

// The live EOL importer turns endoflife.date release cycles into quarantined
// deprecation drafts: a cycle whose EOL date is in the past (or whose eol is the bool
// true) is end-of-life and emitted; a future EOL date or eol:false is NOT yet
// end-of-life and skipped. Output is sorted by signature for deterministic, idempotent
// re-runs.
func TestEOLLive_EmitsPastEOLCyclesAsDeprecationDrafts(t *testing.T) {
	now := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	bodies := map[string]string{
		"python": `[
			{"cycle":"3.8","eol":"2024-10-07"},
			{"cycle":"3.13","eol":"2029-10-01"},
			{"cycle":"2.7","eol":true},
			{"cycle":"3.14","eol":false}
		]`,
	}
	src := ingest.NewEOLLiveSource(
		ingest.WithEOLProducts([]string{"python"}),
		ingest.WithEOLNow(now),
		ingest.WithEOLLiveFetch(eolFixtureFetch(bodies)),
	)
	if src.Name() != "eol-live" {
		t.Errorf("Name() = %q, want eol-live", src.Name())
	}

	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	// 3.8 (past) + 2.7 (eol==true) emitted; 3.13 (future) + 3.14 (false) skipped.
	if got := eolSigs(drafts); len(got) != 2 || got[0] != "EOL:python:2.7" || got[1] != "EOL:python:3.8" {
		t.Fatalf("signatures = %v, want [EOL:python:2.7 EOL:python:3.8] (sorted)", got)
	}

	d := drafts[1] // EOL:python:3.8
	if d.Kind != "fix" {
		t.Errorf("kind = %q, want fix", d.Kind)
	}
	if d.AppliesTo[0].Runtime["python"] != "3.8" {
		t.Errorf("runtime = %v, want python->3.8", d.AppliesTo[0].Runtime)
	}
	if d.SourceLicense != record.SourceLicenseFactsOnly {
		t.Errorf("source_license = %q, want facts-only", d.SourceLicense)
	}
	if !strings.Contains(d.SourceURL, "endoflife.date/python") {
		t.Errorf("source_url = %q", d.SourceURL)
	}
	for _, want := range []string{"end-of-life", "2024-10-07"} {
		if !strings.Contains(d.Symptom.Summary, want) {
			t.Errorf("summary %q missing %q", d.Symptom.Summary, want)
		}
	}
	if d.Resolution == nil || !strings.Contains(d.Resolution.Fix, "supported") {
		t.Errorf("fix should advise upgrading to a supported release: %+v", d.Resolution)
	}
}

// An unknown product (production 404 → nil reader) is skipped, not an error; a
// known product alongside it still yields its drafts.
func TestEOLLive_SkipsUnknownProduct(t *testing.T) {
	now := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	bodies := map[string]string{"go": `[{"cycle":"1.20","eol":"2024-02-06"}]`}
	src := ingest.NewEOLLiveSource(
		ingest.WithEOLProducts([]string{"nonesuch", "go"}),
		ingest.WithEOLNow(now),
		ingest.WithEOLLiveFetch(eolFixtureFetch(bodies)),
	)
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if got := eolSigs(drafts); len(got) != 1 || got[0] != "EOL:go:1.20" {
		t.Fatalf("signatures = %v, want [EOL:go:1.20]", got)
	}
}

// Drafts is deterministic: the same fixture yields byte-identical signatures across
// calls (the precondition for idempotent re-imports under the dedup layer).
func TestEOLLive_DeterministicOrder(t *testing.T) {
	now := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	bodies := map[string]string{"node": `[
		{"cycle":"16","eol":"2023-09-11"},
		{"cycle":"14","eol":"2023-04-30"},
		{"cycle":"18","eol":"2025-04-30"}
	]`}
	mk := func() []string {
		src := ingest.NewEOLLiveSource(
			ingest.WithEOLProducts([]string{"node"}),
			ingest.WithEOLNow(now),
			ingest.WithEOLLiveFetch(eolFixtureFetch(bodies)),
		)
		d, err := src.Drafts(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return eolSigs(d)
	}
	a, b := mk(), mk()
	if strings.Join(a, ",") != strings.Join(b, ",") {
		t.Fatalf("non-deterministic: %v vs %v", a, b)
	}
	if want := "EOL:node:14,EOL:node:16,EOL:node:18"; strings.Join(a, ",") != want {
		t.Fatalf("order = %v, want %s", a, want)
	}
}

// A cycle with an unparseable EOL date is skipped (not emitted with a garbage token),
// and a product returning a malformed 200 body is skipped WITHOUT aborting the batch —
// the bulk-importer "skip junk, never fail the batch" principle (mirrors OSV-live).
func TestEOLLive_SkipsJunk(t *testing.T) {
	now := time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)
	bodies := map[string]string{
		"python": `[{"cycle":"3.8","eol":"2024-10-07"},{"cycle":"3.9","eol":"tba"}]`, // 3.9: junk date
		"broken": `{"message":"upstream error"}`,                                     // malformed 200 (object, not array)
		"go":     `[{"cycle":"1.20","eol":"2024-02-06"}]`,
	}
	src := ingest.NewEOLLiveSource(
		ingest.WithEOLProducts([]string{"python", "broken", "go"}),
		ingest.WithEOLNow(now),
		ingest.WithEOLLiveFetch(eolFixtureFetch(bodies)),
	)
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("a malformed product body must not abort the batch: %v", err)
	}
	// python 3.8 + go 1.20 emitted; python 3.9 (junk date) and broken (malformed body) skipped.
	if got := eolSigs(drafts); len(got) != 2 || got[0] != "EOL:go:1.20" || got[1] != "EOL:python:3.8" {
		t.Fatalf("signatures = %v, want [EOL:go:1.20 EOL:python:3.8]", got)
	}
}

// End-to-end through the shared ingest path: an EOL draft is born quarantined, is
// schema-valid, carries facts-only provenance, and a re-import dedups against it — the
// quarantine + idempotency #0023 requires (the same path the OSV live importer rides).
func TestEOLLive_PrepareQuarantinesAndDedups(t *testing.T) {
	ctx := context.Background()
	src := ingest.NewEOLLiveSource(
		ingest.WithEOLProducts([]string{"python"}),
		ingest.WithEOLNow(time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)),
		ingest.WithEOLLiveFetch(eolFixtureFetch(map[string]string{"python": `[{"cycle":"3.8","eol":"2024-10-07"}]`})),
	)
	drafts, err := src.Drafts(ctx)
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("want 1 draft, got %d", len(drafts))
	}
	d := drafts[0]

	ix := openIx(t)
	meta := ingest.Meta{ID: "exp-0001", Author: "twiceshy-importer", Now: "2026-06-22", IncludeQuarantined: true}
	out, err := ingest.Prepare(ctx, ix, repo, d, meta)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Novelty != index.NoveltyNovel {
		t.Fatalf("first Prepare want Novel, got %q", out.Novelty)
	}
	if out.Record == nil || out.Record.Status != "quarantined" {
		t.Fatalf("first Prepare must quarantine, got record=%v", out.Record)
	}
	if out.Record.Provenance.SourceLicense != record.SourceLicenseFactsOnly {
		t.Errorf("facts-only provenance not carried: %+v", out.Record.Provenance)
	}
	if err := record.Validate(out.Record); err != nil {
		t.Errorf("prepared record not schema-valid: %v", err)
	}

	// Re-import the same draft against an index carrying the quarantined record → dedup.
	if err := ix.Rebuild(ctx, []*record.Record{out.Record}, repo); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	meta.ID = "exp-0002"
	out2, err := ingest.Prepare(ctx, ix, repo, d, meta)
	if err != nil {
		t.Fatalf("Prepare (second): %v", err)
	}
	if out2.Novelty == index.NoveltyNovel {
		t.Fatalf("second Prepare must dedup, got Novel")
	}
	if out2.Record != nil {
		t.Errorf("deduped Prepare must not create another record, got %+v", out2.Record)
	}
}
