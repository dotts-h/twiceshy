package idf

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
)

// buildDocFreq walks each source's directory root recursively, treating one
// file as one document. Unreadable files and binary files (detected via a
// NUL byte in the first 512 bytes) are skipped. Each distinct Tokenize-derived
// token is counted once per document into docFreq, and totalDocs is
// incremented once per counted document.
func buildDocFreq(sources []ManifestSource) (map[string]uint64, uint64, error) {
	docFreq := make(map[string]uint64)
	var totalDocs uint64

	for _, source := range sources {
		walkErr := filepath.WalkDir(source.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Unreadable directory entries are skipped rather than
				// aborting the whole walk.
				return nil
			}
			if d.IsDir() {
				return nil
			}

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}

			if isBinary(data) {
				return nil
			}

			seen := make(map[string]struct{})
			for _, tok := range Tokenize(string(data)) {
				seen[tok] = struct{}{}
			}

			for tok := range seen {
				docFreq[tok]++
			}
			totalDocs++

			return nil
		})
		if walkErr != nil {
			return nil, 0, walkErr
		}
	}

	return docFreq, totalDocs, nil
}

// isBinary reports whether data looks like binary content, detected via the
// presence of a NUL byte within the first 512 bytes.
func isBinary(data []byte) bool {
	n := len(data)
	if n > 512 {
		n = 512
	}
	return bytes.IndexByte(data[:n], 0) != -1
}
