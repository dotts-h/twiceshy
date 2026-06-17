package index_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// FuzzSearchNeverErrors generalizes TestSearchQuoteEscapesFTS5Input (exp-0001)
// from a handful of examples to a property: NO query string — however hostile —
// may make the retrieval path error or panic. Untrusted text must always be
// escaped into a well-formed FTS5 query (ftsQuery). An empty result is a fine
// answer; an error or panic is a security/robustness bug. Search, Retrieve and
// Assess all funnel through the same ftsQuery, so the property covers the whole
// lexical surface.
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

	recs, err := record.LoadCorpus("../..")
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
	})
}
