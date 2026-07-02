package idf

import "sort"

// dfEntry pairs a word with its document frequency.
type dfEntry struct {
	Word string
	DF   uint64
}

// topN returns the top n entries from df sorted by descending document
// frequency, breaking ties lexicographically by word ascending.
func topN(df map[string]uint64, n int) []dfEntry {
	entries := make([]dfEntry, 0, len(df))
	for word, count := range df {
		entries = append(entries, dfEntry{Word: word, DF: count})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].DF != entries[j].DF {
			return entries[i].DF > entries[j].DF
		}
		return entries[i].Word < entries[j].Word
	})

	return entries[:min(n, len(entries))]
}
