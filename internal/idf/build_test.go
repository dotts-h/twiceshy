package idf

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBuildDocFreq_CountsPerDocumentNotPerOccurrence verifies buildDocFreq
// walks each source's directory root recursively, treats one file as one
// document, and counts each distinct Tokenize-derived token once per
// document (not once per occurrence) into docFreq, incrementing totalDocs
// once per counted document. It also verifies that a binary fixture file
// (containing a NUL byte in its first 512 bytes) is skipped entirely: its
// tokens must not appear in docFreq and it must not increment totalDocs.
//
// Fixture layout (internal/idf/testdata/build):
//
//	sourceA/doc1.txt   -> "cat cat cat dog"   (distinct: cat, dog)
//	sourceA/doc2.txt   -> "dog bird"          (distinct: dog, bird)
//	sourceA/sub/doc4.txt -> "eagle eagle"     (distinct: eagle)  [nested dir]
//	sourceB/doc3.txt   -> "cat fish fish fish" (distinct: cat, fish)
//	sourceB/binary.dat -> contains a NUL byte in the first few bytes; must
//	                      be skipped as binary and must not affect counts.
//
// Expected aggregate over both sources:
//
//	docFreq = {cat: 2, dog: 2, bird: 1, eagle: 1, fish: 1}
//	totalDocs = 4 (doc1, doc2, sub/doc4, doc3 -- binary.dat excluded)
func TestBuildDocFreq_CountsPerDocumentNotPerOccurrence(t *testing.T) {
	sources := []ManifestSource{
		{Name: "sourceA", Path: filepath.Join("testdata", "build", "sourceA")},
		{Name: "sourceB", Path: filepath.Join("testdata", "build", "sourceB")},
	}

	docFreq, totalDocs, err := buildDocFreq(sources)
	if err != nil {
		t.Fatalf("buildDocFreq(%v) returned error: %v", sources, err)
	}

	if totalDocs != 4 {
		t.Fatalf("totalDocs = %d, want 4 (binary.dat must be skipped and not counted)", totalDocs)
	}

	// "cat" appears 3 times in doc1.txt and 1 time in doc3.txt. If tokens
	// were counted per-occurrence rather than per-document, df["cat"] would
	// be 4 (3+1) or higher; per-document counting must yield exactly 2
	// (one increment per document that contains "cat" at least once).
	if got := docFreq["cat"]; got != 2 {
		t.Fatalf(`docFreq["cat"] = %d, want 2 (must count once per document despite 3 occurrences in doc1.txt)`, got)
	}

	// "dog" appears once in doc1.txt and once in doc2.txt -> df 2.
	if got := docFreq["dog"]; got != 2 {
		t.Fatalf(`docFreq["dog"] = %d, want 2`, got)
	}

	// "bird" appears only in doc2.txt -> df 1.
	if got := docFreq["bird"]; got != 1 {
		t.Fatalf(`docFreq["bird"] = %d, want 1`, got)
	}

	// "eagle" appears twice in the same nested document (sub/doc4.txt),
	// which must still only count once per document, and also verifies
	// recursive directory walking found the nested file at all.
	if got := docFreq["eagle"]; got != 1 {
		t.Fatalf(`docFreq["eagle"] = %d, want 1 (recursive walk must find sub/doc4.txt, counted once despite 2 occurrences)`, got)
	}

	// "fish" appears three times in doc3.txt -> per-document df must be 1,
	// not 3.
	if got := docFreq["fish"]; got != 1 {
		t.Fatalf(`docFreq["fish"] = %d, want 1 (must count once per document despite 3 occurrences in doc3.txt)`, got)
	}

	// The binary fixture's distinctive token must never appear: binary.dat
	// must be detected via the NUL byte in its first 512 bytes and skipped
	// before tokenization.
	if _, ok := docFreq["zzzoom"]; ok {
		t.Fatalf(`docFreq["zzzoom"] = %d, want token absent entirely (binary.dat must be skipped)`, docFreq["zzzoom"])
	}
}

// TestBuildDocFreq_SkipsUnreadableFile verifies that a file which cannot be
// read (permission denied) is skipped without affecting totalDocs or
// docFreq, and without buildDocFreq returning an error. Permission bits
// cannot be reliably committed to a git fixture (git only tracks the
// executable bit, and CI checkouts may reset modes), so the unreadable file
// is materialized in a temp directory at test time instead.
func TestBuildDocFreq_SkipsUnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: permission bits do not restrict reads, cannot exercise unreadable-file skip")
	}

	dir := t.TempDir()

	readable := filepath.Join(dir, "readable.txt")
	if err := os.WriteFile(readable, []byte("giraffe giraffe zebra"), 0o644); err != nil {
		t.Fatalf("WriteFile(readable) returned error: %v", err)
	}

	unreadable := filepath.Join(dir, "unreadable.txt")
	if err := os.WriteFile(unreadable, []byte("elephant tiger"), 0o644); err != nil {
		t.Fatalf("WriteFile(unreadable) returned error: %v", err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("Chmod(unreadable, 0o000) returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(unreadable, 0o644)
	})

	sources := []ManifestSource{
		{Name: "unreadableSource", Path: dir},
	}

	docFreq, totalDocs, err := buildDocFreq(sources)
	if err != nil {
		t.Fatalf("buildDocFreq(%v) returned error: %v", sources, err)
	}

	// Only readable.txt should have been counted as a document.
	if totalDocs != 1 {
		t.Fatalf("totalDocs = %d, want 1 (unreadable.txt must be skipped, only readable.txt counted)", totalDocs)
	}
	if got := docFreq["giraffe"]; got != 1 {
		t.Fatalf(`docFreq["giraffe"] = %d, want 1`, got)
	}
	if got := docFreq["zebra"]; got != 1 {
		t.Fatalf(`docFreq["zebra"] = %d, want 1`, got)
	}

	// Tokens exclusive to the unreadable file must never appear.
	if _, ok := docFreq["elephant"]; ok {
		t.Fatalf(`docFreq["elephant"] present, want absent entirely (unreadable.txt must be skipped)`)
	}
	if _, ok := docFreq["tiger"]; ok {
		t.Fatalf(`docFreq["tiger"] present, want absent entirely (unreadable.txt must be skipped)`)
	}
}
