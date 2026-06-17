// SPDX-License-Identifier: AGPL-3.0-only

package fingerprint

import (
	"reflect"
	"testing"
)

// The orchestrator's gate for Dedup: classify the fingerprints a record's
// signatures contribute into "already present" vs "new", mirroring the index's
// per-record derivation (each sig -> Generic + App(repo)). Pure, deterministic.
// Expected fingerprints are computed via the real Generic/App so the test tracks
// the algorithm, not frozen hashes.

const repo = "github.com/dotts-h/twiceshy"

func known(fps ...string) map[string]bool {
	m := map[string]bool{}
	for _, f := range fps {
		m[f] = true
	}
	return m
}

func TestDedup_EmptySigs(t *testing.T) {
	r := Dedup(repo, nil, known())
	if len(r.New) != 0 || len(r.Existing) != 0 {
		t.Fatalf("empty sigs: want empty result, got %+v", r)
	}
	// Maps must be non-nil even when empty.
	if r.New == nil || r.Existing == nil {
		t.Fatalf("maps must be non-nil: %+v", r)
	}
}

func TestDedup_AllNew(t *testing.T) {
	r := Dedup(repo, []string{"foo"}, known())
	want := map[string]string{Generic("foo"): "generic", App(repo, "foo"): "app"}
	if !reflect.DeepEqual(r.New, want) {
		t.Fatalf("New: want %v, got %v", want, r.New)
	}
	if len(r.Existing) != 0 {
		t.Fatalf("Existing: want empty, got %v", r.Existing)
	}
}

func TestDedup_AllExisting(t *testing.T) {
	g, a := Generic("foo"), App(repo, "foo")
	r := Dedup(repo, []string{"foo"}, known(g, a))
	want := map[string]string{g: "generic", a: "app"}
	if !reflect.DeepEqual(r.Existing, want) {
		t.Fatalf("Existing: want %v, got %v", want, r.Existing)
	}
	if len(r.New) != 0 {
		t.Fatalf("New: want empty, got %v", r.New)
	}
}

func TestDedup_MixedScopeForOneSig(t *testing.T) {
	// Generic already indexed, App not: the sig is half-known.
	g, a := Generic("foo"), App(repo, "foo")
	r := Dedup(repo, []string{"foo"}, known(g))
	if !reflect.DeepEqual(r.Existing, map[string]string{g: "generic"}) {
		t.Fatalf("Existing: got %v", r.Existing)
	}
	if !reflect.DeepEqual(r.New, map[string]string{a: "app"}) {
		t.Fatalf("New: got %v", r.New)
	}
}

func TestDedup_DuplicateSigsCollapse(t *testing.T) {
	one := Dedup(repo, []string{"foo"}, known())
	two := Dedup(repo, []string{"foo", "foo"}, known())
	if !reflect.DeepEqual(one, two) {
		t.Fatalf("duplicate sigs must collapse: %v vs %v", one, two)
	}
}

func TestDedup_NormalizationCollapse(t *testing.T) {
	// Two sigs that normalize identically (addresses -> <addr>) must collapse to
	// the same fingerprints: 1 generic + 1 app, not 4.
	r := Dedup(repo, []string{"panic at 0xDEAD", "panic at 0xBEEF"}, known())
	if len(r.New) != 2 {
		t.Fatalf("normalization-equal sigs must collapse to 2 fps, got %d: %v", len(r.New), r.New)
	}
	if _, ok := r.New[Generic("panic at 0xDEAD")]; !ok {
		t.Fatalf("expected the shared generic fp present: %v", r.New)
	}
}

func TestDedup_RepoScoping(t *testing.T) {
	// Generic fp is repo-independent; App fp is repo-specific.
	ra := Dedup("repoA", []string{"foo"}, known())
	rb := Dedup("repoB", []string{"foo"}, known())
	if Generic("foo") == "" || ra.New[Generic("foo")] != "generic" || rb.New[Generic("foo")] != "generic" {
		t.Fatalf("generic fp must be repo-independent and present in both")
	}
	if _, ok := ra.New[App("repoB", "foo")]; ok {
		t.Fatalf("repoA result must not contain repoB's app fp")
	}
}

func TestDedup_EmptyRepo(t *testing.T) {
	r := Dedup("", []string{"foo"}, known())
	want := map[string]string{Generic("foo"): "generic", App("", "foo"): "app"}
	if !reflect.DeepEqual(r.New, want) {
		t.Fatalf("empty repo: want %v, got %v", want, r.New)
	}
}

func TestDedup_UnrelatedKnownIgnored(t *testing.T) {
	r := Dedup(repo, []string{"foo"}, known("sha256:deadbeef", Generic("other")))
	if len(r.Existing) != 0 {
		t.Fatalf("unrelated known fps must not appear: %v", r.Existing)
	}
	if len(r.New) != 2 {
		t.Fatalf("want 2 new, got %v", r.New)
	}
}

func TestDedup_Deterministic(t *testing.T) {
	a := Dedup(repo, []string{"foo", "bar"}, known(Generic("bar")))
	b := Dedup(repo, []string{"foo", "bar"}, known(Generic("bar")))
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("non-deterministic: %v vs %v", a, b)
	}
}
