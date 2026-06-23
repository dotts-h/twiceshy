//go:build livecorpus

// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// adversarialVocab is a broad, independently-authored set of common words that
// realistic OFF-DOMAIN prompts (frontend, backend, data, ops, mobile, prose) use.
// It is the threat model for push precision: none of these is a discriminative
// signal, so none may ever be treated as one. Authored from "what off-domain
// sessions actually say", NOT copied from pushStopwords — so this guard is not
// self-fulfilling: a common word that leaks (df in [1,pushMaxDF] and unlisted)
// fails the test by name until it is added to commonWords.
const adversarialVocab = `
component render props hook state store redux context provider react svelte vue angular
template directive binding ref reactive computed watcher lifecycle mount unmount keystroke
button input form label modal dropdown tooltip sidebar navbar layout grid flex css style
theme color font padding margin border shadow animation transition responsive mobile tablet
desktop viewport breakpoint icon image asset bundle webpack vite rollup

backend api endpoint route controller handler middleware request response header cookie
session auth login logout password token jwt oauth permission role server client service
microservice rest graphql websocket grpc payload body status code header query param path

python function method class object value variable string number integer float boolean list
dict array map set tuple loop range index slice append return yield lambda decorator module
import package library framework django flask fastapi pydantic numpy pandas dataframe

data database table column row record schema migration query select insert update delete join
index transaction commit rollback connection pool cache redis postgres mysql mongo sqlite

deploy deployment kubernetes pod replica node cluster container docker image registry helm
ingress service namespace config secret volume mount scale rollout pipeline ci cd build artifact
terraform ansible aws gcp azure lambda bucket queue topic

file folder directory path read write open close stream buffer parse parser format encode decode
test unit assert mock fixture coverage debug log error warning exception trace stack date time
timestamp leap year month day hour minute second timezone parser

refactor rename extract inline cleanup nesting early return guard helper utility wrapper
slow fast performance memory leak optimize profile benchmark latency throughput

recipe changelog release version bump tag branch merge rebase commit push pull clone repository
issue ticket sprint standup review approve

make buy gift mother birthday weather dinner movie book music travel weekend
`

// TestPushGateExcludesCommonVocabulary is the mechanical precision guard: over the
// LIVE corpus, no common word may be discriminative. It catches stoplist under-reach
// that a small hand-picked negative set hides (the failure a reviewer found: "build",
// "data", "function", "value" etc. leaking unlisted). A failure names the leaking word
// and its validated df — add it to commonWords and re-run.
func TestPushGateExcludesCommonVocabulary(t *testing.T) {
	ctx := context.Background()
	recs, err := record.LoadCorpus("../..")
	if err != nil {
		t.Skipf("live corpus unavailable at ../.. (decoupled to twiceshy-corpus, ADR-0021): %v", err)
	}
	ix, err := Open(filepath.Join(t.TempDir(), "vocab.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	if err := ix.Rebuild(ctx, recs, ""); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, w := range strings.Fields(adversarialVocab) {
		if seen[w] {
			continue
		}
		seen[w] = true
		disc, err := ix.discriminativeTokens(ctx, w)
		if err != nil {
			t.Fatal(err)
		}
		if len(disc) > 0 {
			df, _ := ix.validatedDF(ctx, w)
			t.Errorf("common word %q is discriminative (validated df=%d) — add it to commonWords", w, df)
		}
	}
}
