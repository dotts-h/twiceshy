// SPDX-License-Identifier: AGPL-3.0-only

package record_test

import (
	"bytes"
	"reflect"
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
	// STRUCTURAL comparison, not a re-marshal of both sides. Marshaling want and
	// got and byte-comparing is tautological: a *symmetric* drop is invisible —
	// if Marshal omits a field, both sides lose it and still match. The whole
	// point is to catch exactly that (e.g. error_signatures, the dedup key,
	// silently vanishing on the round trip).
	//
	// Compare field-by-field, treating a nil slice/map as equal to an empty one
	// (a pure YAML representation artifact — `error_signatures: []` reloads as
	// nil) while still failing when a *populated* field is lost. Raw is the
	// on-disk byte encoding and legitimately differs, so ignore only it.
	w, g := *want, *got
	w.Raw, g.Raw = nil, nil
	if !valEqualNilEmpty(reflect.ValueOf(w), reflect.ValueOf(g)) {
		mw, _ := record.Marshal(want)
		mg, _ := record.Marshal(got)
		t.Errorf("round-trip changed the record (ignoring Raw):\n--- want ---\n%s\n--- got ---\n%s", mw, mg)
	}
}

// valEqualNilEmpty is reflect.DeepEqual with one relaxation: a nil and an empty
// slice/map compare equal (len 0 == len 0). That distinction is a serialization
// artifact, never data; a length difference (a dropped or added element) still
// fails, which is the property the round-trip test must guard.
func valEqualNilEmpty(a, b reflect.Value) bool {
	if a.Type() != b.Type() {
		return false
	}
	switch a.Kind() {
	case reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return valEqualNilEmpty(a.Elem(), b.Elem())
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			return false
		}
		for i := 0; i < a.Len(); i++ {
			if !valEqualNilEmpty(a.Index(i), b.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if a.Len() != b.Len() {
			return false
		}
		for _, k := range a.MapKeys() {
			bv := b.MapIndex(k)
			if !bv.IsValid() || !valEqualNilEmpty(a.MapIndex(k), bv) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for i := 0; i < a.NumField(); i++ {
			if !valEqualNilEmpty(a.Field(i), b.Field(i)) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a.Interface(), b.Interface())
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
