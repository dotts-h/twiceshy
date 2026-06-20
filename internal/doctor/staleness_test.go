// SPDX-License-Identifier: AGPL-3.0-only

package doctor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotts-h/twiceshy/internal/doctor"
	"github.com/dotts-h/twiceshy/internal/record"
)

var fixedNow = time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)

// stubEOL returns canned cycles per product; an absent product yields nil (the
// real source returns nil on 404), so the doctor skips it.
type stubEOL map[string][]doctor.Cycle

func (s stubEOL) Cycles(_ context.Context, product string) ([]doctor.Cycle, error) {
	return s[product], nil
}

func ptr(s string) *string { return &s }

func recWith(id string, at []record.AppliesTo, until *string) *record.Record {
	return &record.Record{
		ID: id, Kind: "trap", Status: "quarantined", Title: "t",
		AppliesTo:  at,
		Provenance: record.Provenance{Valid: record.Validity{From: "2020-01-01", Until: until}},
		Path:       "experience/2026/" + id + ".md",
	}
}

func ids(r doctor.Report) []string {
	out := []string{}
	for _, f := range r.Findings {
		out = append(out, f.RecordID)
	}
	return out
}

func TestStaleness_FlagsPastValidUntil(t *testing.T) {
	d := doctor.NewStaleness(nil, fixedNow)
	recs := []*record.Record{
		recWith("0001", nil, ptr("2020-01-01")), // past → flag
		recWith("0002", nil, ptr("2099-01-01")), // future → no
		recWith("0003", nil, nil),               // unset → no
	}
	rep, err := d.Run(context.Background(), recs)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(rep); len(got) != 1 || got[0] != "0001" {
		t.Fatalf("valid.until flags = %v, want [0001]", got)
	}
}

func TestStaleness_FlagsEOLFixedCycle(t *testing.T) {
	eol := stubEOL{"go": {{Cycle: "1.16", EOL: "2022-08-01"}, {Cycle: "1.23", EOL: "2099-01-01"}}}
	d := doctor.NewStaleness(eol, fixedNow)
	mk := func(id, fixed string) *record.Record {
		return recWith(id, []record.AppliesTo{{Ecosystem: "Go", Package: "x", Versions: &record.VersionRange{Fixed: ptr(fixed)}}}, nil)
	}
	recs := []*record.Record{
		mk("0001", "1.16.0"), // EOL cycle → flag
		mk("0002", "1.23.0"), // current cycle → no
		mk("0003", "1.99.0"), // no matching cycle → skip
	}
	rep, _ := d.Run(context.Background(), recs)
	if got := ids(rep); len(got) != 1 || got[0] != "0001" {
		t.Fatalf("EOL flags = %v, want [0001]", got)
	}
}

func TestStaleness_SkipsThirdPartyModuleCycleCollision(t *testing.T) {
	// A third-party module's own version must NOT be read as a runtime release
	// cycle just because its major.minor collides with an EOL'd one. kyverno
	// "fixed in v1.16.2" is module version 1.16, not Go 1.16 — flagging it stale
	// because Go 1.16 is EOL is a false positive (regression: OSV import PR #180).
	eol := stubEOL{"go": {{Cycle: "1.16", EOL: "2022-08-01"}, {Cycle: "1.20", EOL: "2024-02-01"}}}
	d := doctor.NewStaleness(eol, fixedNow)
	recs := []*record.Record{
		// domain-qualified module path → its version is a module version, skip.
		recWith("0001", []record.AppliesTo{{Ecosystem: "Go", Package: "github.com/kyverno/kyverno", Versions: &record.VersionRange{Fixed: ptr("1.16.2")}}}, nil),
		// runtime/stdlib record (no domain) on the same EOL cycle → still flagged.
		recWith("0002", []record.AppliesTo{{Ecosystem: "Go", Package: "io/ioutil", Versions: &record.VersionRange{Fixed: ptr("1.16.0")}}}, nil),
	}
	rep, _ := d.Run(context.Background(), recs)
	if got := ids(rep); len(got) != 1 || got[0] != "0002" {
		t.Fatalf("module-collision flags = %v, want [0002] (module skipped, runtime flagged)", got)
	}
}

func TestStaleness_SkipsUnmappedOrNoVersion(t *testing.T) {
	eol := stubEOL{"go": {{Cycle: "1.16", EOL: "2022-08-01"}}}
	d := doctor.NewStaleness(eol, fixedNow)
	recs := []*record.Record{
		// unmapped ecosystem (Maven) → skip even though a version is present
		recWith("0001", []record.AppliesTo{{Ecosystem: "Maven", Package: "a:b", Versions: &record.VersionRange{Fixed: ptr("2.15.0")}}}, nil),
		// mapped, but only Introduced (a deprecation persists) → skip
		recWith("0002", []record.AppliesTo{{Ecosystem: "Go", Package: "io/ioutil", Versions: &record.VersionRange{Introduced: ptr("1.16")}}}, nil),
	}
	rep, _ := d.Run(context.Background(), recs)
	if got := ids(rep); len(got) != 0 {
		t.Fatalf("expected no flags (skip-on-no-data), got %v", got)
	}
}

func TestStaleness_DoesNotMutateRecords(t *testing.T) {
	eol := stubEOL{"go": {{Cycle: "1.16", EOL: "2022-08-01"}}}
	d := doctor.NewStaleness(eol, fixedNow)
	r := recWith("0001", []record.AppliesTo{{Ecosystem: "Go", Versions: &record.VersionRange{Fixed: ptr("1.16.0")}}}, ptr("2020-01-01"))
	_, _ = d.Run(context.Background(), []*record.Record{r})
	if r.Status != "quarantined" || (r.Provenance.Valid.Until != nil && *r.Provenance.Valid.Until != "2020-01-01") {
		t.Fatalf("doctor mutated the record: status=%q until=%v", r.Status, r.Provenance.Valid.Until)
	}
}

func TestStaleness_RealCorpusNotFalseFlagged(t *testing.T) {
	recs, err := record.LoadCorpus("../..")
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	// A populated EOL source for every mapped product — the committed corpus
	// must still not trip signal 2 (its records are conventions/traps without
	// EOL'd runtime Fixed-cycles), nor signal 1 (no past valid.until).
	eol := stubEOL{
		"go":     {{Cycle: "1.16", EOL: "2022-08-01"}, {Cycle: "1.20", EOL: "2024-02-01"}},
		"python": {{Cycle: "3.8", EOL: "2024-10-01"}},
		"nodejs": {{Cycle: "16", EOL: "2023-09-01"}},
	}
	rep, _ := doctor.NewStaleness(eol, fixedNow).Run(context.Background(), recs)
	if len(rep.Findings) != 0 {
		t.Fatalf("committed corpus false-flagged as stale: %v", rep.Findings)
	}
}

func TestEndOfLifeSource_ParsesDateAndBoolEOL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/go.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[{"cycle":"1.23","eol":"2099-01-01"},{"cycle":"1.16","eol":true},{"cycle":"1.24","eol":false}]`))
	}))
	defer srv.Close()
	cycles, err := doctor.NewEndOfLifeSource(srv.URL).Cycles(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range cycles {
		got[c.Cycle] = c.EOL
	}
	if got["1.23"] != "2099-01-01" || got["1.16"] == "" || got["1.24"] != "" {
		t.Fatalf("eol parse = %v (want 1.23 date, 1.16 past-sentinel, 1.24 empty)", got)
	}
	// unknown product → 404 → nil, no error (caller skips)
	if c, err := doctor.NewEndOfLifeSource(srv.URL).Cycles(context.Background(), "nope"); err != nil || c != nil {
		t.Fatalf("unknown product = (%v, %v), want (nil, nil)", c, err)
	}
}
