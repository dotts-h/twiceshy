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

	writeLine := func(format string, args ...any) error {
		if _, err := fmt.Fprintf(gw, format, args...); err != nil {
			gw.Close()
			return err
		}
		return nil
	}

	if err := writeLine("docs\t%d\n", totalDocs); err != nil {
		return err
	}

	for _, e := range entries {
		if err := writeLine("%s\t%d\n", e.Word, e.DF); err != nil {
			return err
		}
	}

	return gw.Close()
}
