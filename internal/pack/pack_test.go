// SPDX-License-Identifier: AGPL-3.0-only

package pack_test

import (
	"testing"

	"github.com/dotts-h/twiceshy/internal/pack"
	"github.com/dotts-h/twiceshy/internal/record"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		license     string
		commercial  bool
		attribution bool
	}{
		{"", true, false}, // twiceshy-authored
		{record.SourceLicenseFactsOnly, true, false}, // distilled facts
		{"MIT", true, false},                         // permissive
		{"Apache-2.0", true, false},                  //
		{"apache-2.0", true, false},                  // case-insensitive
		{"BSD-3-Clause", true, false},                //
		{"BSD-2-Clause", true, false},                //
		{"ISC", true, false},                         //
		{"0BSD", true, false},                        //
		{"CC0-1.0", true, false},                     //
		{"Unlicense", true, false},                   //
		{"CC-BY-4.0", true, true},                    // includable WITH attribution
		{"CC-BY-3.0", true, true},                    //
		{"cc-by-4.0", true, true},                    // case-insensitive
		{"CC-BY-SA-4.0", false, false},               // share-alike — excluded
		{"cc-by-sa-3.0", false, false},               //
		{"CC-BY-NC-4.0", false, false},               // noncommercial — excluded (has cc-by- prefix)
		{"CC-BY-ND-4.0", false, false},               // no-derivatives — excluded
		{"CC-BY-NC-SA-4.0", false, false},            // noncommercial + share-alike — excluded
		{"GPL-3.0-only", false, false},               // copyleft
		{"AGPL-3.0-only", false, false},              //
		{"LGPL-2.1-only", false, false},              //
		{"MPL-2.0", false, false},                    //
		{"EPL-2.0", false, false},                    //
		{"Foo-1.0", false, false},                    // unknown — fail closed
		{"proprietary", false, false},                //
	}
	for _, tt := range tests {
		e := pack.Classify(tt.license)
		if e.Commercial != tt.commercial || e.NeedsAttribution != tt.attribution {
			t.Errorf("Classify(%q) = {commercial:%v attribution:%v}, want {%v %v} (reason: %s)",
				tt.license, e.Commercial, e.NeedsAttribution, tt.commercial, tt.attribution, e.Reason)
		}
		if e.Reason == "" {
			t.Errorf("Classify(%q): empty reason", tt.license)
		}
	}
}

func rec(id, status, license, url string) *record.Record {
	return &record.Record{
		ID:     id,
		Status: status,
		Provenance: record.Provenance{
			SourceLicense: license,
			SourceURL:     url,
		},
	}
}

func TestBuildManifest_CommercialExcludesCopyleftAndAttributesCCBY(t *testing.T) {
	recs := []*record.Record{
		rec("exp-0001", "validated", "MIT", ""),
		rec("exp-0002", "validated", "CC-BY-4.0", "https://github.com/advisories/GHSA-x"),
		rec("exp-0003", "validated", "CC-BY-SA-4.0", "https://example/sa"),
		rec("exp-0004", "validated", "", ""), // twiceshy-authored
		rec("exp-0005", "quarantined", "MIT", ""),
	}
	m := pack.BuildManifest(recs, true /*commercial*/, false /*includeQuarantined*/)

	got := map[string]bool{}
	for _, id := range m.Included {
		got[id] = true
	}
	// MIT, CC-BY (w/ attribution), and authored are in; CC-BY-SA out; quarantined out.
	for _, want := range []string{"exp-0001", "exp-0002", "exp-0004"} {
		if !got[want] {
			t.Errorf("commercial pack should include %s; included=%v", want, m.Included)
		}
	}
	for _, no := range []string{"exp-0003", "exp-0005"} {
		if got[no] {
			t.Errorf("commercial pack must NOT include %s", no)
		}
	}
	// CC-BY-SA excluded for share-alike; quarantined excluded for status.
	reasons := map[string]string{}
	for _, e := range m.Excluded {
		reasons[e.ID] = e.Reason
	}
	if reasons["exp-0003"] == "" || reasons["exp-0005"] == "" {
		t.Errorf("excluded records must carry reasons: %+v", m.Excluded)
	}
	// CC-BY needs attribution; nothing else does.
	if len(m.Attribution) != 1 || m.Attribution[0].ID != "exp-0002" {
		t.Errorf("attribution = %+v, want only exp-0002", m.Attribution)
	}
}

func TestBuildManifest_NonCommercialIncludesAllValidated(t *testing.T) {
	recs := []*record.Record{
		rec("exp-0001", "validated", "GPL-3.0-only", ""), // copyleft ok in a non-commercial pack
		rec("exp-0002", "validated", "CC-BY-SA-4.0", ""), //
		rec("exp-0003", "quarantined", "MIT", ""),        // still excluded (not validated)
	}
	m := pack.BuildManifest(recs, false /*commercial*/, false)
	if len(m.Included) != 2 {
		t.Errorf("non-commercial pack should include both validated records; included=%v", m.Included)
	}
}

func TestBuildManifest_IncludeQuarantined(t *testing.T) {
	recs := []*record.Record{rec("exp-0001", "quarantined", "MIT", "")}
	if m := pack.BuildManifest(recs, false, false); len(m.Included) != 0 {
		t.Errorf("quarantined excluded by default; included=%v", m.Included)
	}
	if m := pack.BuildManifest(recs, false, true); len(m.Included) != 1 {
		t.Errorf("-include-quarantined should include it; included=%v", m.Included)
	}
}
