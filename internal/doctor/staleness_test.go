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
		ID: id, Kind: "trap", Status: "validated", Title: "t",
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
	if r.Status != "validated" || (r.Provenance.Valid.Until != nil && *r.Provenance.Valid.Until != "2020-01-01") {
		t.Fatalf("doctor mutated the record: status=%q until=%v", r.Status, r.Provenance.Valid.Until)
	}
}

// Staleness demotes validated→stale, so it evaluates ONLY validated records. A
// quarantined draft on an EOL runtime (e.g. an imported advisory for Python 3.8)
// must NOT be flagged — only its validated copy is a demotion candidate.
// Regression: the live osv-live importer piled up ~15 un-mergeable PRs because
// quarantined EOL-runtime advisories tripped the D2 guard test, freezing the corpus
// at 745 records (2026-06-22).
func TestStaleness_EvaluatesOnlyValidated(t *testing.T) {
	eol := stubEOL{"python": {{Cycle: "3.8", EOL: "2024-10-01"}}}
	d := doctor.NewStaleness(eol, fixedNow)
	at := []record.AppliesTo{{Ecosystem: "PyPI", Versions: &record.VersionRange{Fixed: ptr("3.8")}}}
	quar := recWith("0001", at, nil)
	quar.Status = "quarantined"
	val := recWith("0002", at, nil) // recWith defaults to validated
	rep, _ := d.Run(context.Background(), []*record.Record{quar, val})
	if got := ids(rep); len(got) != 1 || got[0] != "0002" {
		t.Fatalf("staleness flags = %v, want [0002] (quarantined draft exempt, validated flagged)", got)
	}
}

// WouldFlag is the promote-side mirror of the D2 guard (#0071): it reports the
// staleness finding a record WOULD receive if it were validated, independent of
// its current status. The promoter calls it to refuse a born-stale advisory (an
// EOL runtime) while it is still quarantined — Run, gated on validated status,
// would not yet flag it, so the only thing that catches it post-promotion is the
// guard test that reds the validate PR. WouldFlag closes that gap pre-promotion.
func TestStaleness_WouldFlag_StatusIndependent(t *testing.T) {
	eol := stubEOL{"python": {{Cycle: "3.8", EOL: "2024-10-01"}}}
	s := doctor.NewStaleness(eol, fixedNow)
	at := []record.AppliesTo{{Ecosystem: "PyPI", Versions: &record.VersionRange{Fixed: ptr("3.8")}}}

	quar := recWith("0001", at, nil)
	quar.Status = "quarantined"
	// Run skips the quarantined draft (status gate) ...
	if rep, _ := s.Run(context.Background(), []*record.Record{quar}); len(ids(rep)) != 0 {
		t.Fatalf("Run must skip the quarantined draft, got %v", ids(rep))
	}
	// ... but WouldFlag reports it as born-stale (the gate's signal).
	if f := s.WouldFlag(context.Background(), quar); f == nil {
		t.Fatal("WouldFlag must report a quarantined EOL-runtime record as born-stale")
	}

	// A current runtime is not flagged (the gate must not over-block).
	live := recWith("0002", []record.AppliesTo{{Ecosystem: "PyPI", Versions: &record.VersionRange{Fixed: ptr("3.13")}}}, nil)
	if f := s.WouldFlag(context.Background(), live); f != nil {
		t.Fatalf("WouldFlag must not flag a current runtime, got %+v", f)
	}
	// Signal 1 (past valid.until) is reported too, status notwithstanding.
	expired := recWith("0003", nil, ptr("2020-01-01"))
	expired.Status = "quarantined"
	if f := s.WouldFlag(context.Background(), expired); f == nil {
		t.Fatal("WouldFlag must report a past valid.until as born-stale")
	}
}

func TestStaleness_RealCorpusNotFalseFlagged(t *testing.T) {
	recs, err := record.LoadCorpus("../..")
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	// A populated EOL source for every mapped product — the committed corpus's
	// VALIDATED records must not trip signal 2 (EOL'd runtime Fixed-cycle) nor
	// signal 1 (past valid.until). Quarantined imports (incl. EOL-runtime
	// advisories) are exempt: the doctor evaluates only validated records.
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
