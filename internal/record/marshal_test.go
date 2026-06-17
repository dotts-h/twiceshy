package record_test

import (
	"bytes"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

// The orchestrator's gate for Marshal: serializing a Record back to markdown
// must round-trip through Parse exactly. This is the write path's persistence
// primitive — what ingest produces in memory has to land on disk and re-load
// identically. Property: Parse(r.Path, Marshal(r)) equals r (modulo Raw, which
// is the on-disk bytes and legitimately differs between the two encodings).

func eqIgnoringRaw(t *testing.T, want, got *record.Record) {
	t.Helper()
	// Compare the canonical serialized form, not reflect.DeepEqual: nil vs empty
	// slice/map is a yaml representation artifact we don't care about. A stable
	// on-disk form (and a body/frontmatter that re-marshal identically) is the
	// property that matters; a dropped required field is caught by Parse failing.
	mw, err := record.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	mg, err := record.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	if !bytes.Equal(mw, mg) {
		t.Errorf("round-trip not stable:\n--- want ---\n%s\n--- got ---\n%s", mw, mg)
	}
}

func TestMarshal_RoundTripsCorpus(t *testing.T) {
	recs, err := record.LoadCorpus("../..")
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	if len(recs) == 0 {
		t.Fatal("empty corpus — nothing to round-trip")
	}
	for _, r := range recs {
		out, err := record.Marshal(r)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", r.ID, err)
		}
		back, err := record.Parse(r.Path, out)
		if err != nil {
			t.Fatalf("re-Parse(%s): %v\n--- marshaled ---\n%s", r.ID, err, out)
		}
		eqIgnoringRaw(t, r, back)
	}
}

func TestMarshal_OutputIsFencedWithBody(t *testing.T) {
	recs, err := record.LoadCorpus("../..")
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	out, err := record.Marshal(recs[0])
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("---\n")) {
		t.Errorf("output must open with a frontmatter fence, got: %.20q", out)
	}
	if !bytes.Contains(out, []byte("\n---\n")) {
		t.Errorf("output must close the frontmatter fence")
	}
	if !bytes.Contains(out, []byte(recs[0].Body)) {
		t.Errorf("output must contain the narrative body")
	}
}

// A freshly-quarantined record (optional fields nil — no validated_at, no guard,
// no usage) must round-trip with the nils preserved, not materialized.
func TestMarshal_QuarantinedNilsRoundTrip(t *testing.T) {
	src := []byte(`---
schema_version: 1
id: exp-0042
kind: convention
status: quarantined
title: Prefer constructor injection over package globals
applies_to:
  - ecosystem: Go
    package: example.com/thing
provenance:
  source: { author: "claude", session: "sess-1", pr: null }
  recorded_at: 2026-06-17
  validated_at: null
  valid: { from: 2026-06-17, until: null }
  superseded_by: null
---

Use constructors; do not reach for package-level mutable state.
`)
	r, err := record.Parse("experience/2026/0042-prefer-constructor-injection.md", src)
	if err != nil {
		t.Fatalf("fixture parse: %v", err)
	}
	out, err := record.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := record.Parse(r.Path, out)
	if err != nil {
		t.Fatalf("re-Parse: %v\n%s", err, out)
	}
	if back.Provenance.ValidatedAt != nil || back.Guard != nil {
		t.Errorf("nil optionals must stay nil after round-trip: validated_at=%v guard=%v",
			back.Provenance.ValidatedAt, back.Guard)
	}
	eqIgnoringRaw(t, r, back)
}
