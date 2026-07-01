// SPDX-License-Identifier: AGPL-3.0-only

package repro

import (
	"strings"
	"testing"
)

func TestCapWriter_BoundsOutput(t *testing.T) {
	w := newCapWriter(10)
	// Write 1 MiB in chunks; only 10 bytes may be retained.
	chunk := strings.Repeat("x", 4096)
	for i := 0; i < 256; i++ {
		n, err := w.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("Write returned n=%d err=%v; must always report full consumption", n, err)
		}
	}
	got := w.String()
	if !strings.HasPrefix(got, "xxxxxxxxxx") {
		t.Errorf("kept %q, want the first 10 bytes", got)
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("output past the cap must be marked truncated; got %q", got)
	}
	if len(got) > 200 {
		t.Errorf("retained %d bytes; the cap must bound host memory", len(got))
	}
}

func TestCapWriter_UnderCapNotTruncated(t *testing.T) {
	w := newCapWriter(1024)
	_, _ = w.Write([]byte("short output"))
	if got := w.String(); got != "short output" {
		t.Errorf("got %q, want %q (no truncation marker under the cap)", got, "short output")
	}
}

// Writing exactly the cap's worth of bytes drops nothing, so it must not be
// marked truncated — the boundary between "fits exactly" and "one byte over".
func TestCapWriter_ExactCapNotTruncated(t *testing.T) {
	w := newCapWriter(10)
	_, _ = w.Write([]byte("0123456789")) // exactly 10 bytes
	if got := w.String(); got != "0123456789" {
		t.Errorf("got %q, want %q (nothing was dropped at the exact cap)", got, "0123456789")
	}
}

// One byte over the cap is the boundary's other side: exactly one byte is
// dropped, so it must be marked truncated.
func TestCapWriter_OneByteOverCapIsTruncated(t *testing.T) {
	w := newCapWriter(10)
	_, _ = w.Write([]byte("01234567891")) // 11 bytes, 1 over
	got := w.String()
	if !strings.HasSuffix(got, "truncated by twiceshy broker...]") {
		t.Errorf("got %q, want a truncation marker (1 byte was dropped)", got)
	}
	if !strings.HasPrefix(got, "0123456789") {
		t.Errorf("got %q, want the first 10 bytes retained", got)
	}
}
