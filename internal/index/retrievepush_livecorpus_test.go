//go:build livecorpus

// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"slices"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
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
	// The reproduced live specimen (#0106/ADR-0028): its only discriminative
	// tokens ("application", "llm") each live in a DIFFERENT unrelated validated
	// record — the cross-record false positive #0108's corroboration rule closes.
	"need a deep analysis of this application and why it is still not working well not helping any llm",
	// #0107: exp-0003 is kind convention and exp-0043 is twiceshy-importer origin —
	// neither is push-eligible anymore, however exact the topical match. They stay
	// reachable via pull (search_experience); see TestRetrievePushExcludesQuarantined's
	// sibling invariant, and the eligibility unit tests in eligibility_test.go.
	"mcp streamable http session id",
	"io ioutil deprecated",
}

// pushOnTopic error prompts must inject the genuinely-relevant card: wantID
// present, and where today's blanket push leaks the weak exp-0005 card, absent
// names it so a regression that re-adds noise fails loudly. Each query carries
// at least two co-occurring discriminative tokens (#0108: prompt-triggered push
// now requires two, not one).
var pushOnTopic = []struct {
	query  string
	wantID string
	absent string // a noise id that must NOT appear ("" = no assertion)
}{
	{"fts5 syntax error on dotted query", "exp-0001", "exp-0005"},
	{"go test fails with permission denied on a noexec TMPDIR", "exp-0017", "exp-0005"},
	{"bm25 negative scores", "exp-0002", ""},
	{"servemux method pattern catch-all", "exp-0006", ""},
	{"nonroot bind mount permission denied", "exp-0004", ""},
	{"forgejo setup-go cache hang", "exp-0005", ""},
}

// TestRetrievePushPrecisionRecall is the push-channel relevance guard. It runs
// against the live corpus (record.LoadCorpus) so a future record that closes the
// document-frequency gap fails here rather than silently re-noising push. It
// asserts INVARIANTS (off-topic==0; on-topic contains the right id, noise absent),
// not exact counts, so ordinary corpus growth does not make it brittle.
// Moved to livecorpus because the push gate's BM25 floor (pushFloor=3.0) is
// corpus-scale-dependent and cannot be satisfied by the small fixture corpus.
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

// TestRetrievePushTraced exposes the gate decision for telemetry (#0067) without
// changing what RetrievePush serves: the discriminative tokens that opened the
// gate and the served hits. Off-topic queries leave the decision empty.
// Moved to livecorpus because the push gate floor is corpus-scale-dependent.
func TestRetrievePushTraced(t *testing.T) {
	ix := openIndex(t, corpus(t))
	ctx := context.Background()

	t.Run("discriminative query records its gate tokens + served hits", func(t *testing.T) {
		const q = "bm25 negative scores" // not an exact signature → discriminative path
		d, err := ix.RetrievePushTraced(ctx, index.Query{Text: q})
		if err != nil {
			t.Fatal(err)
		}
		if d.FingerprintBypass {
			t.Error("a non-signature query must not record a fingerprint bypass")
		}
		if !slices.Contains(d.Discriminative, "bm25") {
			t.Errorf("gate tokens should include the discriminative term: %v", d.Discriminative)
		}
		if !hasID(hitIDs(d.Served), "exp-0002") {
			t.Errorf("served should include exp-0002: %v", hitIDs(d.Served))
		}
		// The trace must serve EXACTLY what RetrievePush serves — no behavioral drift.
		plain, err := ix.RetrievePush(ctx, index.Query{Text: q})
		if err != nil {
			t.Fatal(err)
		}
		if len(plain) != len(d.Served) {
			t.Errorf("wrapper drift: RetrievePush served %d, traced served %d", len(plain), len(d.Served))
		}
	})

	t.Run("off-topic query records an empty decision", func(t *testing.T) {
		d, err := ix.RetrievePushTraced(ctx, index.Query{Text: "write a haiku about cats"})
		if err != nil {
			t.Fatal(err)
		}
		if d.FingerprintBypass || len(d.Discriminative) != 0 || len(d.Served) != 0 {
			t.Errorf("off-topic must be an empty decision: %+v", d)
		}
	})
}

// TestRetrievePushExcludesQuarantined pins two invariants at once: quarantined
// records never reach the push channel, and document-frequency is counted over
// VALIDATED records only — so a token living only in a quarantined OSV advisory
// is non-discriminative and injects nothing. Moved to livecorpus because it
// depends on a specific quarantined record from the live corpus.
func TestRetrievePushExcludesQuarantined(t *testing.T) {
	ix := openIndex(t, corpus(t))
	hits, err := ix.RetrievePush(context.Background(), index.Query{Text: "libheif strukturag heif image vulnerability"})
	if err != nil {
		t.Fatalf("RetrievePush: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("quarantined-only query injected %v, want 0", hitIDs(hits))
	}
}

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
