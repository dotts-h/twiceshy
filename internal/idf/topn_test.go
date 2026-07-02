package idf

import "testing"

func TestTopN(t *testing.T) {
	df := map[string]uint64{
		"apple":      10,
		"banana":     50,
		"cherry":     50,
		"date":       5,
		"elderberry": 25,
		"fig":        1,
		"grape":      50,
	}

	got := topN(df, 4)

	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4", len(got))
	}

	want := []dfEntry{
		{Word: "banana", DF: 50},
		{Word: "cherry", DF: 50},
		{Word: "grape", DF: 50},
		{Word: "elderberry", DF: 25},
	}

	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}
