package idf

import (
	"compress/gzip"
	"fmt"
	"io"
)

// writeTable gzip-writes a TSV table to w: a "docs\t<totalDocs>" header line
// followed by one "word\t<df>" line per entry, in the given slice order.
func writeTable(w io.Writer, totalDocs uint64, entries []dfEntry) error {
	gw := gzip.NewWriter(w)

	if _, err := fmt.Fprintf(gw, "docs\t%d\n", totalDocs); err != nil {
		gw.Close()
		return err
	}

	for _, e := range entries {
		if _, err := fmt.Fprintf(gw, "%s\t%d\n", e.Word, e.DF); err != nil {
			gw.Close()
			return err
		}
	}

	return gw.Close()
}
