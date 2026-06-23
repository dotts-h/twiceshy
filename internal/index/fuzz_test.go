// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/testcorpus"
)

// FuzzSearchNeverErrors generalizes TestSearchQuoteEscapesFTS5Input (exp-0001)
// from a handful of examples to a property: NO query string — however hostile —
// may make the retrieval path error or panic. Untrusted text must always be
// escaped into a well-formed FTS5 query (ftsQuery). An empty result is a fine
// answer; an error or panic is a security/robustness bug. Search, Retrieve and
// Assess funnel through ftsQuery; RetrievePush adds the push gate's per-token
// validated-DF path (ftsPhrase), which the error-pull hook (#0087) makes
// load-bearing — so the property now covers the whole lexical surface, /push
// included.
func FuzzSearchNeverErrors(f *testing.F) {
	for _, s := range []string{
		"", "   ", "index",
		`modernc.org/sqlite`, `utf-8 node.js`,
		`"unbalanced quote`, `AND OR NOT NEAR(`, `-col:^prefix*`,
		`. - / " ( ) *`, `'); DROP TABLE records; --`,
		`a OR (b AND "c`, `***`, "\x00\x01\x02", "café   ☃ \t\n", `NEAR/5 foo`,
	} {
		f.Add(s)
	}
	for _, s := range fieldReportErrorLines { // verbatim error lines the hook sends (#0087)
		f.Add(s)
	}

	recs, err := record.LoadCorpus(testcorpus.Root())
	if err != nil {
		f.Fatalf("LoadCorpus: %v", err)
	}
	ix, err := index.Open(filepath.Join(f.TempDir(), "ix.db"))
	if err != nil {
		f.Fatalf("Open: %v", err)
	}
	f.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(context.Background(), recs, "github.com/dotts-h/twiceshy"); err != nil {
		f.Fatalf("Rebuild: %v", err)
	}

	f.Fuzz(func(t *testing.T, text string) {
		ctx := context.Background()
		if _, err := ix.Search(ctx, index.Query{Text: text}); err != nil {
			t.Fatalf("Search(%q) errored: %v", text, err)
		}
		if _, err := ix.Retrieve(ctx, index.Query{Text: text}); err != nil {
			t.Fatalf("Retrieve(%q) errored: %v", text, err)
		}
		if _, err := ix.Assess(ctx, index.Query{Text: text}); err != nil {
			t.Fatalf("Assess(%q) errored: %v", text, err)
		}
		if _, err := ix.RetrievePush(ctx, index.Query{Text: text}); err != nil {
			t.Fatalf("RetrievePush(%q) errored: %v", text, err)
		}
	})
}
