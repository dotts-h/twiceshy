// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

func readWtfFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("data", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func stubWtf(bodies map[string]string) ingest.WtfOption {
	return ingest.WithWtfFetch(func(_ context.Context, target string) (io.ReadCloser, error) {
		body, ok := bodies[target]
		if !ok {
			return nil, nil
		}
		return io.NopCloser(strings.NewReader(body)), nil
	})
}

func TestWtf_Name(t *testing.T) {
	if got := ingest.NewWtfSource().Name(); got != "wtf" {
		t.Errorf("Name = %q, want wtf", got)
	}
}

func TestWtf_ParseFixtures(t *testing.T) {
	src := ingest.NewWtfSource(stubWtf(map[string]string{
		"wtfjs":     readWtfFixture(t, "wtfjs_fixture.md"),
		"wtfpython": readWtfFixture(t, "wtfpython_fixture.md"),
	}))
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	// 4 valid wtfjs + 3 valid wtfpython; malformed entries (missing explanation) skipped.
	if len(drafts) != 7 {
		t.Fatalf("want 7 drafts (4 wtfjs + 3 wtfpython), got %d", len(drafts))
	}

	var wtfjs, wtfpython int
	bySig := map[string]ingest.Draft{}
	for _, d := range drafts {
		bySig[d.Symptom.ErrorSignatures[0]] = d
		if strings.HasPrefix(d.Symptom.ErrorSignatures[0], "wtfjs:") {
			wtfjs++
		}
		if strings.HasPrefix(d.Symptom.ErrorSignatures[0], "wtfpython:") {
			wtfpython++
		}
	}
	if wtfjs != 4 || wtfpython != 3 {
		t.Fatalf("ecosystem counts: wtfjs=%d wtfpython=%d", wtfjs, wtfpython)
	}

	emptyArr, ok := bySig["wtfjs:-is-equal-"]
	if !ok {
		t.Fatalf("missing wtfjs [] == ![] draft: %+v", drafts)
	}
	if emptyArr.Kind != "trap" {
		t.Errorf("kind = %q, want trap", emptyArr.Kind)
	}
	if emptyArr.Title != "`[]` is equal `![]`" {
		t.Errorf("title = %q", emptyArr.Title)
	}
	if !strings.Contains(emptyArr.Symptom.Summary, "Array is equal not array") {
		t.Errorf("summary = %q", emptyArr.Symptom.Summary)
	}
	if !strings.Contains(emptyArr.Body, "[] == ![]") || !strings.Contains(emptyArr.Body, "abstract equality operator") {
		t.Errorf("body missing snippet or explanation: %q", emptyArr.Body)
	}
	if emptyArr.Resolution == nil || !strings.Contains(emptyArr.Resolution.RootCause, "abstract equality operator") {
		t.Errorf("root_cause = %+v", emptyArr.Resolution)
	}
	if emptyArr.Resolution == nil || strings.TrimSpace(emptyArr.Resolution.Fix) == "" {
		t.Errorf("fix must be non-empty: %+v", emptyArr.Resolution)
	}
	if len(emptyArr.AppliesTo) != 1 || emptyArr.AppliesTo[0].Ecosystem != "npm" || emptyArr.AppliesTo[0].Package != "" {
		t.Errorf("applies_to = %+v, want npm with empty package", emptyArr.AppliesTo)
	}
	if emptyArr.SourceLicense != "WTFPL" {
		t.Errorf("source_license = %q, want WTFPL", emptyArr.SourceLicense)
	}
	wantURL := "https://github.com/denysdovhan/wtfjs/blob/master/README.md#-is-equal-"
	if emptyArr.SourceURL != wantURL {
		t.Errorf("source_url = %q, want %q", emptyArr.SourceURL, wantURL)
	}

	nan, ok := bySig["wtfjs:nan-is-not-a-nan"]
	if !ok {
		t.Fatalf("missing NaN draft")
	}
	if !strings.Contains(nan.Resolution.Fix, "accept it as it is") {
		t.Errorf("fix should quote entry guidance: %q", nan.Resolution.Fix)
	}

	chained, ok := bySig["wtfpython:be-careful-with-chained-operations"]
	if !ok {
		t.Fatalf("missing chained-operations draft")
	}
	if len(chained.AppliesTo) != 1 || chained.AppliesTo[0].Ecosystem != "PyPI" {
		t.Errorf("applies_to = %+v, want PyPI", chained.AppliesTo)
	}
	wantPyURL := "https://github.com/satwikkansal/wtfpython/blob/master/README.md#-be-careful-with-chained-operations"
	if chained.SourceURL != wantPyURL {
		t.Errorf("source_url = %q, want %q", chained.SourceURL, wantPyURL)
	}
	if !strings.Contains(chained.Body, "False == False in [False]") {
		t.Errorf("body missing python snippet: %q", chained.Body)
	}

	hash, ok := bySig["wtfpython:hash-brownies"]
	if !ok {
		t.Fatalf("missing hash brownies draft")
	}
	if !strings.Contains(hash.Resolution.Fix, "delete the key") {
		t.Errorf("fix should carry entry-stated workaround: %q", hash.Resolution.Fix)
	}

	for _, sig := range []string{"wtfjs:-malformed-missing-explanation-", "wtfpython:-malformed-missing-explanation-"} {
		if _, found := bySig[sig]; found {
			t.Errorf("malformed entry %q must be skipped", sig)
		}
	}
}

func TestWtf_SkipsMalformedMissingExplanation(t *testing.T) {
	const body = `# 👀 Examples

## Valid entry

Intro line:

` + "```js\n1;\n```" + `

### 💡 Explanation:

Because reasons.

## Malformed missing explanation

` + "```js\n2;\n```" + `

# 📚 Other resources
`
	src := ingest.NewWtfSource(stubWtf(map[string]string{"wtfjs": body}))
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("want 1 draft (malformed skipped), got %d", len(drafts))
	}
	if drafts[0].Title != "Valid entry" {
		t.Errorf("title = %q", drafts[0].Title)
	}
}

func TestWtf_FetchErrorFailsBatch(t *testing.T) {
	wantErr := errors.New("boom")
	src := ingest.NewWtfSource(ingest.WithWtfFetch(func(_ context.Context, _ string) (io.ReadCloser, error) {
		return nil, wantErr
	}))
	if _, err := src.Drafts(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
}

func TestWtf_DeterministicOrder(t *testing.T) {
	fixtures := map[string]string{
		"wtfjs":     readWtfFixture(t, "wtfjs_fixture.md"),
		"wtfpython": readWtfFixture(t, "wtfpython_fixture.md"),
	}
	mk := func() []string {
		src := ingest.NewWtfSource(stubWtf(fixtures))
		d, err := src.Drafts(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var sigs []string
		for _, draft := range d {
			sigs = append(sigs, draft.Symptom.ErrorSignatures[0])
		}
		return sigs
	}
	a, b := mk(), mk()
	if strings.Join(a, ",") != strings.Join(b, ",") {
		t.Fatalf("non-deterministic: %v vs %v", a, b)
	}
}

func TestWtf_CapsEntriesPerRun(t *testing.T) {
	var b strings.Builder
	b.WriteString("# 👀 Examples\n\n")
	for i := 0; i < ingest.MaxWtfDrafts+50; i++ {
		fmt.Fprintf(&b, "## entry%03d\n\n```js\n%d; // -> %d\n```\n\n### 💡 Explanation:\n\nBecause %d.\n\n", i, i, i, i)
	}
	src := ingest.NewWtfSource(stubWtf(map[string]string{"wtfjs": b.String()}))
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != ingest.MaxWtfDrafts {
		t.Fatalf("want %d drafts (capped), got %d", ingest.MaxWtfDrafts, len(drafts))
	}
}

func TestWtf_PrepareQuarantinesAndDedups(t *testing.T) {
	ctx := context.Background()
	src := ingest.NewWtfSource(stubWtf(map[string]string{
		"wtfjs": readWtfFixture(t, "wtfjs_fixture.md"),
	}))
	drafts, err := src.Drafts(ctx)
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 4 {
		t.Fatalf("want 4 wtfjs drafts, got %d", len(drafts))
	}
	d := drafts[0]

	ix := openIx(t)
	meta := ingest.Meta{ID: "exp-0001", Author: "twiceshy-importer", Now: "2026-07-08", IncludeQuarantined: true}
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
	if out.Record.Kind != "trap" {
		t.Errorf("kind = %q, want trap", out.Record.Kind)
	}
	if out.Record.Provenance.Source.Author != "twiceshy-importer" {
		t.Errorf("author = %q", out.Record.Provenance.Source.Author)
	}
	if out.Record.Provenance.SourceLicense != "WTFPL" || out.Record.Provenance.SourceURL == "" {
		t.Errorf("provenance not carried: %+v", out.Record.Provenance)
	}
	if err := record.Validate(out.Record); err != nil {
		t.Errorf("prepared record not schema-valid: %v", err)
	}

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
}
