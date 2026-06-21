// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// pushOffTopic prompts must inject NOTHING: their content tokens are either
// absent from the validated corpus (df=0), too generic (df>=maxDF), or an
// ecosystem name. No discriminative token clears the push gate, so the push
// channel stays silent. This is the bug being fixed: today's blanket push
// injects 2-3 cards on each of these (measured live on /push).
var pushOffTopic = []string{
	"what time is it in Tokyo",
	"write a haiku about cats",
	"how do I center a div in CSS",
	"what is the capital of France",
	"the quick brown fox jumps",
	"how do I implement a feature",
	"explain how this works",                // guards the df=1 prose-token leak ("works")
	"test the code",                         // guards generic df>=3 tokens
	"go to the store and buy milk",          // pure off-topic prose
	"docker is great for shipping software", // guards the ecosystem-name stoplist
}

// pushOnTopic error prompts must inject the genuinely-relevant card: wantID
// present, and where today's blanket push leaks the weak exp-0005 card, absent
// names it so a regression that re-adds noise fails loudly.
var pushOnTopic = []struct {
	query  string
	wantID string
	absent string // a noise id that must NOT appear ("" = no assertion)
}{
	{"fts5 syntax error on dotted query", "exp-0001", "exp-0005"},
	{"go test fails with permission denied in TMPDIR", "exp-0017", "exp-0005"},
	{"bm25 negative scores", "exp-0002", ""},
	{"mcp streamable http session id", "exp-0003", ""},
	{"servemux method pattern fallthrough", "exp-0006", ""},
	{"io ioutil deprecated", "exp-0043", ""},
	{"nonroot bind mount permission denied", "exp-0004", ""},
	{"forgejo setup-go cache hang", "exp-0005", ""},
}

// TestRetrievePushPrecisionRecall is the push-channel relevance guard. It runs
// against the live corpus (record.LoadCorpus) so a future record that closes the
// document-frequency gap fails here rather than silently re-noising push. It
// asserts INVARIANTS (off-topic==0; on-topic contains the right id, noise absent),
// not exact counts, so ordinary corpus growth does not make it brittle.
func TestRetrievePushPrecisionRecall(t *testing.T) {
	ix := openIndex(t, corpus(t))
	ctx := context.Background()

	for _, q := range pushOffTopic {
		hits, err := ix.RetrievePush(ctx, index.Query{Text: q})
		if err != nil {
			t.Fatalf("RetrievePush(%q): %v", q, err)
		}
		if len(hits) != 0 {
			t.Errorf("off-topic %q injected %d card(s) %v, want 0", q, len(hits), hitIDs(hits))
		}
	}

	for _, c := range pushOnTopic {
		hits, err := ix.RetrievePush(ctx, index.Query{Text: c.query})
		if err != nil {
			t.Fatalf("RetrievePush(%q): %v", c.query, err)
		}
		got := hitIDs(hits)
		if !hasID(got, c.wantID) {
			t.Errorf("on-topic %q -> %v, want %s present", c.query, got, c.wantID)
		}
		if c.absent != "" && hasID(got, c.absent) {
			t.Errorf("on-topic %q -> %v, noise %s must be absent", c.query, got, c.absent)
		}
	}
}

// TestRetrievePushExcludesQuarantined pins two invariants: quarantined records
// never reach the push channel, and document-frequency is counted over VALIDATED
// records only — so a discriminative token living only in a quarantined record is
// non-discriminative (df=0 over validated) and injects nothing.
//
// It runs against a CONTROLLED fixture, not the live corpus. The previous version
// targeted a real record (exp-0010 libheif) assuming it would "stay quarantined",
// but the autonomous panel correctly promoted it to validated — and a test coupled
// to mutable corpus state then silently blocked every promotion batch from merging.
func TestRetrievePushExcludesQuarantined(t *testing.T) {
	q := mustParseFixture(t, "experience/2099/9001-zqx-quarantined-fixture.md", quarantinedFixtureMD)
	v := mustParseFixture(t, "experience/2099/9002-zqx-validated-fixture.md", validatedFixtureMD)
	ix := openIndex(t, []*record.Record{q, v})
	// "zqxbarnaclium" lives ONLY in the quarantined record: rare enough to clear the
	// discriminative gate if it were validated, yet it must inject nothing because
	// push excludes quarantined records and counts df over validated only.
	hits, err := ix.RetrievePush(context.Background(), index.Query{Text: "zqxbarnaclium quarantined fixture vulnerability"})
	if err != nil {
		t.Fatalf("RetrievePush: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("a token living only in a quarantined record injected %v, want 0", hitIDs(hits))
	}
}

func mustParseFixture(t *testing.T, path, md string) *record.Record {
	t.Helper()
	rec, err := record.Parse(path, []byte(md))
	if err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
	return rec
}

const quarantinedFixtureMD = `---
schema_version: 1
id: exp-9001
kind: trap
status: quarantined
title: 'zqxbarnaclium: quarantined fixture advisory'
symptom:
    summary: 'zqxbarnaclium quarantined fixture advisory for the push-exclusion test'
    error_signatures:
        - ZQXBARNACLIUM-0001
applies_to:
    - ecosystem: Go
      package: example.com/zqxbarnaclium
      versions:
        introduced: "0"
        fixed: 1.0.0
resolution:
    root_cause: Synthetic quarantined record; its zqxbarnaclium token lives nowhere else.
    fix: Upgrade past the fixed version.
provenance:
    source:
        author: twiceshy-test
        session: null
        pr: null
    recorded_at: "2099-01-01"
    validated_at: null
    valid:
        from: "2099-01-01"
        until: null
    source_license: CC-BY-4.0
    source_url: https://example.com/zqxbarnaclium
    superseded_by: null
---
zqxbarnaclium quarantined fixture advisory body — the distinctive token appears only here.
`

const validatedFixtureMD = `---
schema_version: 1
id: exp-9002
kind: trap
status: validated
title: frobnicatorium validated fixture trap
symptom:
    summary: a validated fixture trap with its own frobnicatorium vocabulary
    error_signatures:
        - "frobnicatorium-9002"
applies_to:
    - ecosystem: Go
      package: example.com/frobnicatorium
resolution:
    root_cause: synthetic validated record so document-frequency has a validated corpus.
    fix: the validated fix for the frobnicatorium trap.
guard:
    guarding_test: "TestFrobnicatoriumFixture"
provenance:
    source:
        author: twiceshy-test
        session: test
    recorded_at: "2099-01-01"
    validated_at: "2099-01-01"
    valid:
        from: "2099-01-01"
---
A validated fixture record body with the frobnicatorium vocabulary so the index has a validated document.
`

func hitIDs(hits []index.Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.ID
	}
	return out
}

func hasID(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}
