// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
)

// fieldReportErrorLines are verbatim error lines from the RN/iOS field report
// that motivated the error-scoped retrieval trigger (#0087). The hook hands the
// search path the raw line, which is dense with FTS5-hostile punctuation: a
// dotted module path, a scoped npm package, a CocoaPods "[!]" marker, quotes and
// colons. exp-0001 (tokenize + quote every token, never hand raw text to MATCH)
// is what keeps these from being parsed as FTS5 syntax. Shared by the named
// guard below and seeded into FuzzSearchNeverErrors so the same lines pin both.
var fieldReportErrorLines = []string{
	`TypeError: Cannot read property 'lngLat' of null`,
	`[!] Unable to find a specification for 'RCT-Folly' depended upon by 'RNMapboxMaps'`,
	`error: package @scope/pkg@1.2.3 failed to resolve`,
	`panic: runtime error: invalid memory address modernc.org/sqlite`,
	`SyntaxError: Unexpected token '<' in JSON at position 0`,
	`Traceback (most recent call last): File "app.py", line 1, in <module>`,
	`fatal: node.js ENOENT: no such file or directory, open './x'`,
}

// TestErrorScopedQueriesSurviveHostileInput is #0087's blocking server
// prerequisite: the retrieval seams the error-pull hook drives — RetrievePush
// (the /push endpoint) and Search (search_experience) — must SURVIVE a verbatim
// error line, not just a hand-built query. An empty result is a fine answer; an
// error or panic out of the FTS5 MATCH parser is the bug the hook would trip on
// for real (exp-0001). RetrievePush is not exercised by FuzzSearchNeverErrors,
// so this is the only guard on the push path against error-shaped input.
func TestErrorScopedQueriesSurviveHostileInput(t *testing.T) {
	ctx := context.Background()
	ix := openIndex(t, corpus(t))
	for _, line := range fieldReportErrorLines {
		q := index.Query{Text: line}
		if _, err := ix.RetrievePush(ctx, q); err != nil {
			t.Errorf("RetrievePush(%q) errored: %v", line, err)
		}
		if _, err := ix.Search(ctx, q); err != nil {
			t.Errorf("Search(%q) errored: %v", line, err)
		}
	}
}
