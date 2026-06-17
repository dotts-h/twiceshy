// SPDX-License-Identifier: AGPL-3.0-only

package index_test

import (
	"context"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// NextID allocates the next exp-NNNN id from the highest currently indexed,
// zero-padded to four digits. Allocation is MAX+1, not count+1, so gaps in the
// corpus never cause a collision; an empty corpus starts at exp-0001.
func TestNextID(t *testing.T) {
	t.Run("empty corpus starts at exp-0001", func(t *testing.T) {
		ix := openIndex(t, nil)
		got, err := ix.NextID(context.Background())
		if err != nil {
			t.Fatalf("NextID: %v", err)
		}
		if got != "exp-0001" {
			t.Errorf("empty corpus: got %q, want exp-0001", got)
		}
	})

	t.Run("returns max+1, zero-padded", func(t *testing.T) {
		ix := openIndex(t, []*record.Record{
			mkRecord(t, 1, "alpha record one", "the first seeded record body", nil, "Go", "a"),
			mkRecord(t, 2, "bravo record two", "the second seeded record body", nil, "Go", "b"),
			mkRecord(t, 3, "charlie record three", "the third seeded record body", nil, "Go", "c"),
		})
		got, err := ix.NextID(context.Background())
		if err != nil {
			t.Fatalf("NextID: %v", err)
		}
		if got != "exp-0004" {
			t.Errorf("got %q, want exp-0004", got)
		}
	})

	// The id is derived from the maximum, not the row count: a corpus with gaps
	// (e.g. superseded records removed) must still never reuse an id.
	t.Run("non-contiguous ids use max+1, not count+1", func(t *testing.T) {
		ix := openIndex(t, []*record.Record{
			mkRecord(t, 1, "alpha record one", "the first seeded record body", nil, "Go", "a"),
			mkRecord(t, 42, "foxtrot record forty two", "the forty-second seeded record body", nil, "Go", "f"),
		})
		got, err := ix.NextID(context.Background())
		if err != nil {
			t.Fatalf("NextID: %v", err)
		}
		if got != "exp-0043" {
			t.Errorf("got %q, want exp-0043 (max+1, not count+1)", got)
		}
	})
}
