// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// A fingerprint-exact match on the push channel bypasses the discriminative-term
// gate because a deterministic stack signature is real context by construction
// (ADR-0015). That bypass must serve ONLY the fingerprint hit(s): it does NOT
// license admitting extra BM25 hits drawn from the full query, which
// Retrieve->Search would append to fill k. Here a second record lexically echoes
// the query hard enough to clear pushFloor, so a bypass that fell through to
// Retrieve would leak it as a lexical card.
func TestRetrievePushBypassServesOnlyFingerprintHits(t *testing.T) {
	ctx := context.Background()
	sig := "frobnicator overload at sector seven"

	// recA owns the signature -> the exact query fingerprint-hits it.
	recA := mkRecord(t, 101, "frobnicator overload trap", "the frobnicator overloads", []string{sig}, "Go", "frob")
	// recB carries no signature (no competing fingerprint) but its summary echoes
	// the query's tokens heavily, so its BM25 against the full query is strong.
	echo := strings.TrimSpace(strings.Repeat(sig+" ", 40))
	recB := mkRecord(t, 102, "frobnicator overload sector notes", echo, nil, "Go", "frob")

	recs := []*record.Record{recA, recB}
	for i := 0; i < 10; i++ {
		recs = append(recs, mkRecord(t, 110+i, "unrelated filler", "cache eviction retry budget", nil, "Go", "frob"))
	}
	ix := openIndex(t, recs)

	dec, err := ix.RetrievePushTraced(ctx, index.Query{Text: sig})
	if err != nil {
		t.Fatalf("RetrievePushTraced: %v", err)
	}
	if !dec.FingerprintBypass {
		t.Fatalf("query is the exact signature — expected a fingerprint bypass, got %+v", dec)
	}
	for _, h := range dec.Served {
		if h.Matched != index.MatchedFingerprint {
			t.Errorf("fingerprint bypass served a non-fingerprint hit %s (matched=%q) — the bypass must not admit lexical fill", h.ID, h.Matched)
		}
		if h.ID == "exp-0102" {
			t.Errorf("fingerprint bypass leaked lexical near-miss exp-0102")
		}
	}
}
