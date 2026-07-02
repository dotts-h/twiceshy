package idf

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Table holds document-frequency counts parsed from a decompressed TSV
// stream: a "docs\t<N>" header line giving the total document count,
// followed by "word\t<df>" rows giving the per-word document frequency.
type Table struct {
	totalDocs uint64
	df        map[string]uint64
}

// parseTable reads a decompressed TSV stream consisting of a "docs\t<N>"
// header line followed by zero or more "word\t<df>" rows.
func parseTable(r io.Reader) (*Table, error) {
	scanner := bufio.NewScanner(r)

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("idf: empty table stream, expected docs header")
	}

	header := scanner.Text()
	headerFields := strings.SplitN(header, "\t", 2)
	if len(headerFields) != 2 || headerFields[0] != "docs" {
		return nil, fmt.Errorf("idf: invalid table header %q, want \"docs\\t<N>\"", header)
	}
	totalDocs, err := strconv.ParseUint(headerFields[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("idf: invalid docs count in header %q: %w", header, err)
	}

	table := &Table{
		totalDocs: totalDocs,
		df:        make(map[string]uint64),
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 {
			return nil, fmt.Errorf("idf: invalid table row %q, want \"word\\t<df>\"", line)
		}
		df, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("idf: invalid df count in row %q: %w", line, err)
		}
		table.df[strings.ToLower(fields[0])] = df
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return table, nil
}

// Available reports whether the table has any word/df rows loaded.
func (t *Table) Available() bool {
	return t != nil && len(t.df) > 0
}

// TotalDocs returns the total document count from the table header.
func (t *Table) TotalDocs() uint64 {
	if t == nil {
		return 0
	}
	return t.totalDocs
}

// DF returns the document frequency for word, case-insensitively, and
// whether the word was present in the table.
func (t *Table) DF(word string) (uint64, bool) {
	if t == nil {
		return 0, false
	}
	df, ok := t.df[strings.ToLower(word)]
	return df, ok
}
