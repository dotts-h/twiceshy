// SPDX-License-Identifier: AGPL-3.0-only

package doctor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

// findOne returns the (single) Finding for id, or fails. It lets a test assert on
// the human-actionable Issue/Proposal payload, not just which record was flagged.
func findOne(t *testing.T, r doctor.Report, id string) doctor.Finding {
	t.Helper()
	for _, f := range r.Findings {
		if f.RecordID == id {
			return f
		}
	}
	t.Fatalf("no finding for %s in %v", id, r.Findings)
	return doctor.Finding{}
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
	// The Issue must carry the offending date and the signal-1 wording; the
	// Proposal must propose the stale transition. A bug that emitted the wrong
	// signal's text or dropped the date would pass an ids()-only check.
	f := findOne(t, rep, "0001")
	if !strings.Contains(f.Issue, "2020-01-01") || !strings.Contains(f.Issue, "in the past") {
		t.Fatalf("signal-1 Issue = %q, want the date + \"in the past\"", f.Issue)
	}
	if !strings.Contains(f.Proposal, "set status: stale") {
		t.Fatalf("signal-1 Proposal = %q, want it to propose the stale transition", f.Proposal)
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
	// Signal-2's Issue names the EOL cycle; its Proposal carries the
	// signal-2-distinct "out of use" wording (both signals' proposals contain
	// "stale", so keying only on "stale" would be a weak assertion). A swap of the
	// two signals' messages would pass an ids()-only check.
	f := findOne(t, rep, "0001")
	if !strings.Contains(f.Issue, "end-of-life") || !strings.Contains(f.Issue, "1.16") {
		t.Fatalf("signal-2 Issue = %q, want \"end-of-life\" + the cycle", f.Issue)
	}
	if !strings.Contains(f.Proposal, "out of use") {
		t.Fatalf("signal-2 Proposal = %q, want the \"out of use\" wording", f.Proposal)
	}
}

// When BOTH signals fire (a past valid.until AND an EOL Fixed cycle), signal 1
// wins — wouldFlag returns on the valid.until match before staleByEOL runs
// (staleness.go early-return order). Pins that the reported finding is signal 1's.
func TestStaleness_PastValidUntilWinsOverEOL(t *testing.T) {
	eol := stubEOL{"go": {{Cycle: "1.16", EOL: "2022-08-01"}}}
	d := doctor.NewStaleness(eol, fixedNow)
	// Empty Package → runtime, so signal 2 (EOL 1.16) would also fire on its own.
	r := recWith("0001", []record.AppliesTo{{Ecosystem: "Go", Versions: &record.VersionRange{Fixed: ptr("1.16.0")}}}, ptr("2020-01-01"))
	rep, _ := d.Run(context.Background(), []*record.Record{r})
	if got := ids(rep); len(got) != 1 || got[0] != "0001" {
		t.Fatalf("both-signals flags = %v, want [0001]", got)
	}
	f := findOne(t, rep, "0001")
	if !strings.Contains(f.Issue, "valid.until") {
		t.Fatalf("both signals present: want the valid.until message (signal 1 wins), got %q", f.Issue)
	}
}

// The eol:true sentinel end-to-end: endoflife normalizes `eol:true` (already EOL,
// no date) to the literal "0001-01-01" (endoflife.go normalized()). Pin that
// staleByEOL actually FLAGS a Cycle carrying that sentinel — the exact "definitely
// past, no date" case the sentinel exists for. The record uses a runtime package
// (io/ioutil) so isRuntimePackage returns true; a domain path would be skipped.
func TestStaleness_FlagsEOLTrueSentinel(t *testing.T) {
	eol := stubEOL{"go": {{Cycle: "1.16", EOL: "0001-01-01"}}} // == endoflife.go's eol:true sentinel
	d := doctor.NewStaleness(eol, fixedNow)
	rec := recWith("0001", []record.AppliesTo{{Ecosystem: "Go", Package: "io/ioutil", Versions: &record.VersionRange{Fixed: ptr("1.16.0")}}}, nil)
	rep, _ := d.Run(context.Background(), []*record.Record{rec})
	if got := ids(rep); len(got) != 1 || got[0] != "0001" {
		t.Fatalf("eol:true-sentinel flags = %v, want [0001]", got)
	}
	f := findOne(t, rep, "0001")
	if !strings.Contains(f.Issue, "end-of-life") {
		t.Fatalf("sentinel finding Issue = %q, want the signal-2 end-of-life message", f.Issue)
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

func TestEndOfLifeSource_ParsesDateAndBoolEOL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/go.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[{"cycle":"1.23","eol":"2099-01-01"},{"cycle":"1.16","eol":true},{"cycle":"1.24","eol":false},{"cycle":"1.10","eol":null},{"cycle":"1.11","eol":""}]`))
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
	// A JSON null or an empty-string eol is "unknown", NOT "already EOL" — flagging
	// a current runtime stale on missing data is fail-unsafe (would demote a live one).
	if got["1.10"] != "" || got["1.11"] != "" {
		t.Fatalf("eol null/empty = %v (want both empty/not-EOL)", map[string]string{"1.10": got["1.10"], "1.11": got["1.11"]})
	}
	// unknown product → 404 → nil, no error (caller skips)
	if c, err := doctor.NewEndOfLifeSource(srv.URL).Cycles(context.Background(), "nope"); err != nil || c != nil {
		t.Fatalf("unknown product = (%v, %v), want (nil, nil)", c, err)
	}
}

// The endoflife base URL is operator-configurable, so the response body is bounded.
// A source streaming past the cap is truncated mid-JSON and fails the decode rather
// than buffering an unbounded body into memory.
func TestEndOfLifeSource_BoundsResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A single cycle whose eol string is far larger than the 1 MiB cap.
		_, _ = w.Write([]byte(`[{"cycle":"1","eol":"` + strings.Repeat("a", 2<<20) + `"}]`))
	}))
	defer srv.Close()
	if _, err := doctor.NewEndOfLifeSource(srv.URL).Cycles(context.Background(), "go"); err == nil {
		t.Fatal("oversized response decoded without error — body is not bounded")
	}
}
