// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// A long off-topic push query is mostly non-discriminative tokens that never grow
// the discriminative set. The gate must still bound its serial validated-DF
// round-trips so an authenticated client can't amplify one query into one SQLite
// round-trip per distinct token.
func TestDiscriminativeTokens_BoundsDFRoundTrips(t *testing.T) {
	ix, err := Open(filepath.Join(t.TempDir(), "ix.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = ix.Close() }()

	var sb strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&sb, "tokenword%03d ", i)
	}
	var calls int
	df := func(_ context.Context, _ string) (int, error) { calls++; return 0, nil }

	out, err := ix.discriminativeTokensVia(context.Background(), sb.String(), df)
	if err != nil {
		t.Fatalf("discriminativeTokensVia: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("want no discriminative tokens, got %v", out)
	}
	if calls > maxQueryTokens {
		t.Errorf("validated-DF round-trips = %d, want <= %d", calls, maxQueryTokens)
	}
}
