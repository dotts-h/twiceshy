// SPDX-License-Identifier: AGPL-3.0-only

package selfaudit_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/selfaudit"
)

func TestParseGoMod(t *testing.T) {
	const gomod = `module github.com/dotts-h/twiceshy

go 1.25.0

require (
	github.com/google/jsonschema-go v0.4.3
	modernc.org/sqlite v1.52.0
)

require gopkg.in/yaml.v3 v3.0.1

require (
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
)

replace example.com/old => example.com/new v1.0.0
`
	deps, err := selfaudit.ParseGoMod(strings.NewReader(gomod))
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}
	got := map[string]string{}
	for _, d := range deps {
		got[d.Path] = d.Version
	}
	want := map[string]string{
		"github.com/google/jsonschema-go": "v0.4.3",
		"modernc.org/sqlite":              "v1.52.0",
		"gopkg.in/yaml.v3":                "v3.0.1",
		"golang.org/x/mod":                "v0.33.0", // indirect deps are audited too — vulns hide there
		"golang.org/x/sys":                "v0.39.0",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d deps %v, want %d %v", len(got), got, len(want), want)
	}
	for p, v := range want {
		if got[p] != v {
			t.Errorf("%s: got %q, want %q", p, got[p], v)
		}
	}
	// The module's own path and the replace directive's RHS are not deps.
	if _, ok := got["github.com/dotts-h/twiceshy"]; ok {
		t.Error("the module's own path must not be a dep")
	}
	if _, ok := got["example.com/new"]; ok {
		t.Error("a replace RHS must not be parsed as a require")
	}
}

// ParseGoMod hand-parses and claims to skip module/go/replace/exclude/retract/
// toolchain directives. TestParseGoMod only proves module+replace are skipped;
// this pins the rest — a parseRequire that accepted 2-field non-require lines
// would wrongly ingest `exclude foo v1.0.0` or `replace`. Also exercises the
// len(f)<2 drop branch and require-block edges (empty block, inline comment).
func TestParseGoMod_SkipsNonRequireDirectives(t *testing.T) {
	tests := map[string]struct {
		gomod string
		want  map[string]string
	}{
		"top-level non-require directives are skipped": {
			gomod: "module github.com/dotts-h/twiceshy\n" +
				"go 1.25.0\n" +
				"toolchain go1.25.0\n" +
				"retract v1.2.3\n" +
				"exclude foo v1.0.0\n" +
				"require foo v1.0.0\n",
			// Only the genuine require survives: toolchain/retract/exclude/go all dropped.
			want: map[string]string{"foo": "v1.0.0"},
		},
		"single-field require line is dropped (parseRequire len<2)": {
			gomod: "require (\n" +
				"\tbarewordnoversion\n" +
				"\tfoo v1.0.0\n" +
				")\n",
			want: map[string]string{"foo": "v1.0.0"},
		},
		"empty require block yields no deps": {
			gomod: "require (\n)\n",
			want:  map[string]string{},
		},
		"inline comment on the require opener still opens the block": {
			gomod: "require ( // a comment after the opener\n" +
				"\tfoo v1.0.0\n" +
				")\n",
			want: map[string]string{"foo": "v1.0.0"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			deps, err := selfaudit.ParseGoMod(strings.NewReader(tc.gomod))
			if err != nil {
				t.Fatalf("ParseGoMod: %v", err)
			}
			got := map[string]string{}
			for _, d := range deps {
				got[d.Path] = d.Version
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d deps %v, want %d %v", len(got), got, len(tc.want), tc.want)
			}
			for p, v := range tc.want {
				if got[p] != v {
					t.Errorf("%s: got %q, want %q", p, got[p], v)
				}
			}
			// None of the skipped directives' tokens may leak in as a dep path.
			for _, leaked := range []string{"toolchain", "go1.25.0", "retract", "exclude", "barewordnoversion", "go"} {
				if _, ok := got[leaked]; ok {
					t.Errorf("non-require token %q leaked in as a dep: %v", leaked, got)
				}
			}
		})
	}
}

func TestAudit(t *testing.T) {
	deps := []selfaudit.Dep{
		{Path: "github.com/google/jsonschema-go", Version: "v0.4.3"},
		{Path: "modernc.org/sqlite", Version: "v1.52.0"},
	}

	t.Run("surfaces a dep inside an affected range", func(t *testing.T) {
		recs := []*record.Record{adv(t, "exp-9001", "GHSA-aaaa-bbbb-cccc", "Go", "github.com/google/jsonschema-go", "0", strptr("0.5.0"))}
		hits := selfaudit.Audit(deps, recs)
		if len(hits) != 1 {
			t.Fatalf("got %d hits, want 1: %+v", len(hits), hits)
		}
		if hits[0].RecordID != "exp-9001" || hits[0].AdvisoryID != "GHSA-aaaa-bbbb-cccc" {
			t.Errorf("hit = %+v", hits[0])
		}
		if hits[0].Dep.Path != "github.com/google/jsonschema-go" {
			t.Errorf("wrong dep: %+v", hits[0].Dep)
		}
	})

	t.Run("catches a MAL- malicious-package advisory (broader than IsAdvisoryClass)", func(t *testing.T) {
		// record.IsAdvisoryClass recognizes only GHSA/CVE/GO; a security monitor
		// that reused it would miss MAL- (malicious packages) in its own deps.
		recs := []*record.Record{adv(t, "exp-9006", "MAL-2025-1234", "Go", "github.com/google/jsonschema-go", "0", nil)}
		hits := selfaudit.Audit(deps, recs)
		if len(hits) != 1 || hits[0].AdvisoryID != "MAL-2025-1234" {
			t.Fatalf("a MAL- advisory on a Go dep must be flagged; got %+v", hits)
		}
	})

	t.Run("no hit when the version is at or past the fixed version", func(t *testing.T) {
		recs := []*record.Record{adv(t, "exp-9002", "GHSA-dddd-eeee-ffff", "Go", "modernc.org/sqlite", "0", strptr("1.50.0"))}
		if hits := selfaudit.Audit(deps, recs); len(hits) != 0 {
			t.Fatalf("v1.52.0 >= fixed 1.50.0 must not be flagged; got %+v", hits)
		}
	})

	t.Run("a pre-release at the fixed version is still flagged (fail-safe)", func(t *testing.T) {
		// v1.50.0-rc1 is BEFORE the fix 1.50.0 in semver, so it is affected — a
		// monitor that dropped the -rc and called it fixed would miss a real vuln.
		pre := []selfaudit.Dep{{Path: "modernc.org/sqlite", Version: "v1.50.0-rc1"}}
		recs := []*record.Record{adv(t, "exp-9007", "GHSA-pre0-rele-ase0", "Go", "modernc.org/sqlite", "0", strptr("1.50.0"))}
		if hits := selfaudit.Audit(pre, recs); len(hits) != 1 {
			t.Fatalf("v1.50.0-rc1 is before the fix and must be flagged; got %+v", hits)
		}
	})

	t.Run("an unfixed advisory (fixed null) flags any version at or above introduced", func(t *testing.T) {
		recs := []*record.Record{adv(t, "exp-9003", "GHSA-gggg-hhhh-iiii", "Go", "modernc.org/sqlite", "1.0.0", nil)}
		if hits := selfaudit.Audit(deps, recs); len(hits) != 1 {
			t.Fatalf("v1.52.0 >= introduced 1.0.0 with no fix must be flagged; got %+v", hits)
		}
	})

	t.Run("exactly the introduced version is flagged (inclusive lower bound)", func(t *testing.T) {
		// The affected range is introduced <= v < fixed, so v == introduced is IN it.
		// affected() hinges on cmpVer(v, introduced) < 0 returning false here; an
		// off-by-one flipping `<` to `<=` would MISS a vuln at exactly introduced —
		// the fail-unsafe direction a security monitor must never take.
		local := []selfaudit.Dep{{Path: "modernc.org/sqlite", Version: "v1.10.0"}}
		recs := []*record.Record{adv(t, "exp-9008", "GHSA-intr-oduc-edxx", "Go", "modernc.org/sqlite", "1.10.0", nil)}
		if hits := selfaudit.Audit(local, recs); len(hits) != 1 {
			t.Fatalf("v1.10.0 == introduced 1.10.0 must be flagged (inclusive lower bound); got %+v", hits)
		}
	})

	t.Run("one patch below introduced is not flagged (below the affected range)", func(t *testing.T) {
		local := []selfaudit.Dep{{Path: "modernc.org/sqlite", Version: "v1.9.9"}}
		recs := []*record.Record{adv(t, "exp-9009", "GHSA-belo-wint-rodx", "Go", "modernc.org/sqlite", "1.10.0", nil)}
		if hits := selfaudit.Audit(local, recs); len(hits) != 0 {
			t.Fatalf("v1.9.9 < introduced 1.10.0 must NOT be flagged; got %+v", hits)
		}
	})

	t.Run("exactly the fixed version is not flagged (exclusive upper bound)", func(t *testing.T) {
		// introduced "0" so the lower bound never excludes; only the fixed boundary
		// decides. v == fixed is OUT of the range (introduced <= v < fixed), so
		// cmpVer(v, fixed) >= 0 must hold — a fail-safe (false-positive) boundary.
		local := []selfaudit.Dep{{Path: "modernc.org/sqlite", Version: "v1.52.0"}}
		recs := []*record.Record{adv(t, "exp-9010", "GHSA-fixe-dexa-ctxx", "Go", "modernc.org/sqlite", "0", strptr("1.52.0"))}
		if hits := selfaudit.Audit(local, recs); len(hits) != 0 {
			t.Fatalf("v1.52.0 == fixed 1.52.0 must NOT be flagged (exclusive upper bound); got %+v", hits)
		}
	})

	t.Run("ignores a non-Go advisory even when the package string matches", func(t *testing.T) {
		recs := []*record.Record{adv(t, "exp-9004", "GHSA-jjjj-kkkk-llll", "npm", "github.com/google/jsonschema-go", "0", strptr("99.0.0"))}
		if hits := selfaudit.Audit(deps, recs); len(hits) != 0 {
			t.Fatalf("npm ecosystem must not match a Go module; got %+v", hits)
		}
	})

	t.Run("ignores a non-advisory record (no vuln id)", func(t *testing.T) {
		dep := &record.Record{ // a deprecation-shaped record: matching package+range but NO GHSA id
			ID: "exp-9005", Kind: "trap", Status: "quarantined", Title: "io/ioutil deprecated",
			Symptom:   &record.Symptom{Summary: "deprecated", ErrorSignatures: []string{"SA1019"}},
			AppliesTo: []record.AppliesTo{{Ecosystem: "Go", Package: "github.com/google/jsonschema-go", Versions: &record.VersionRange{Introduced: strptr("0"), Fixed: strptr("99.0.0")}}},
		}
		if hits := selfaudit.Audit(deps, []*record.Record{dep}); len(hits) != 0 {
			t.Fatalf("a non-advisory (no vuln id) must not be a security hit; got %+v", hits)
		}
	})
}

func strptr(s string) *string { return &s }

func adv(t *testing.T, id, vulnID, eco, pkg, introduced string, fixed *string) *record.Record {
	t.Helper()
	intro := introduced
	return &record.Record{
		ID: id, Kind: "trap", Status: "quarantined",
		Title:     vulnID + " in " + pkg,
		Symptom:   &record.Symptom{ErrorSignatures: []string{vulnID}},
		AppliesTo: []record.AppliesTo{{Ecosystem: eco, Package: pkg, Versions: &record.VersionRange{Introduced: &intro, Fixed: fixed}}},
	}
}
