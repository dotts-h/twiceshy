// SPDX-License-Identifier: AGPL-3.0-only

package author_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/author"
	"github.com/dotts-h/twiceshy/internal/record"
)

var fixedNow = time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)

func scaffold(t *testing.T, p author.Params) map[string]string {
	t.Helper()
	files, err := author.Scaffold(p, fixedNow)
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	out := map[string]string{}
	for _, f := range files {
		out[f.Path] = f.Content
	}
	return out
}

func paths(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The scaffolded record must parse as a valid quarantined, §5-clean authored
// record — so the author can fill in the blanks and `doctor revalidate` straight
// away, never fighting the schema.
func TestScaffoldRecordParsesAsQuarantinedAuthored(t *testing.T) {
	files := scaffold(t, author.Params{ID: "exp-0091", Slug: "my-trap", Title: "a trap about X", Kind: "trap", Author: "claude"})
	const recPath = "experience/2026/0091-my-trap.md"
	md, ok := files[recPath]
	if !ok {
		t.Fatalf("no record at %s; got %v", recPath, paths(files))
	}
	rec, err := record.Parse(recPath, []byte(md))
	if err != nil {
		t.Fatalf("scaffolded record must parse:\n%v\n---\n%s", err, md)
	}
	if rec.Status != "quarantined" {
		t.Errorf("status = %q, want quarantined", rec.Status)
	}
	if rec.Provenance.SourceLicense != record.SourceLicenseAuthoredInternal {
		t.Errorf("source_license = %q, want the authored-internal sentinel", rec.Provenance.SourceLicense)
	}
	if rec.Provenance.SourceURL != "" {
		t.Errorf("an authored record must carry no source_url, got %q", rec.Provenance.SourceURL)
	}
	if rec.Provenance.Source.Author != "claude" {
		t.Errorf("source.author = %q, want claude", rec.Provenance.Source.Author)
	}
	if rec.Title != "a trap about X" {
		t.Errorf("title = %q", rec.Title)
	}
}

// The record's guard must reference the generated positive repro skeleton, so the
// record + its proof are wired together from the start.
func TestScaffoldEmitsPositiveReproReferencedByTheRecord(t *testing.T) {
	files := scaffold(t, author.Params{ID: "exp-0091", Slug: "my-trap", Title: "a trap about quasar buffers", Kind: "trap", Author: "claude"})
	const reproPath = "experience/repro/0091-my-trap.sh"
	body, ok := files[reproPath]
	if !ok {
		t.Fatalf("no positive repro at %s; got %v", reproPath, paths(files))
	}
	if !strings.HasPrefix(body, "#!") {
		t.Errorf("repro skeleton must start with a shebang, got:\n%s", body)
	}
	rec, err := record.Parse("experience/2026/0091-my-trap.md", []byte(files["experience/2026/0091-my-trap.md"]))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Guard == nil || len(rec.Guard.Repros) != 1 ||
		rec.Guard.Repros[0].Path != reproPath || rec.Guard.Repros[0].Kind != "positive" {
		t.Errorf("guard.repros must reference %s as positive; got %+v", reproPath, rec.Guard)
	}
}

// -with-negative adds a negative repro skeleton and wires it into guard.repros.
func TestScaffoldWithNegativeEmitsNegativeRepro(t *testing.T) {
	files := scaffold(t, author.Params{ID: "exp-0091", Slug: "my-trap", Title: "a trap about quasar buffers", Kind: "trap", Author: "claude", WithNegative: true})
	const negPath = "experience/repro/0091-my-trap-negative.sh"
	if _, ok := files[negPath]; !ok {
		t.Fatalf("no negative repro at %s; got %v", negPath, paths(files))
	}
	rec, err := record.Parse("experience/2026/0091-my-trap.md", []byte(files["experience/2026/0091-my-trap.md"]))
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, r := range rec.Guard.Repros {
		kinds = append(kinds, r.Kind)
	}
	if len(kinds) != 2 {
		t.Errorf("want positive + negative repros, got %v", kinds)
	}
}

func TestScaffoldRejectsBadParams(t *testing.T) {
	const ok = "a valid title here"
	cases := map[string]author.Params{
		"bad id":      {ID: "0091", Slug: "x", Title: ok, Author: "claude"},
		"bad slug":    {ID: "exp-0091", Slug: "Bad Slug", Title: ok, Author: "claude"},
		"short title": {ID: "exp-0091", Slug: "x", Title: "short", Author: "claude"},
		"bad kind":    {ID: "exp-0091", Slug: "x", Title: ok, Kind: "bogus", Author: "claude"},
		"no author":   {ID: "exp-0091", Slug: "x", Title: ok},
	}
	for name, p := range cases {
		if _, err := author.Scaffold(p, fixedNow); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
