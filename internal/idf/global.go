package idf

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"sync"
)

//go:embed table.tsv.gz
var tableGz []byte

var (
	globalOnce  sync.Once
	globalTable *Table
)

// Global returns the process-wide idf Table, lazily gzip-decompressing and
// parsing the embedded table.tsv.gz asset exactly once. Subsequent calls
// return the same cached *Table instance.
func Global() *Table {
	globalOnce.Do(func() {
		globalTable = mustLoadGlobal()
	})
	return globalTable
}

func mustLoadGlobal() *Table {
	gz, err := gzip.NewReader(bytes.NewReader(tableGz))
	if err != nil {
		panic("idf: failed to open embedded table.tsv.gz: " + err.Error())
	}
	defer gz.Close()

	t, err := parseTable(gz)
	if err != nil {
		panic("idf: failed to parse embedded table.tsv.gz: " + err.Error())
	}
	return t
}
