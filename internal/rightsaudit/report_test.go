// SPDX-License-Identifier: AGPL-3.0-only

package rightsaudit_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/pack"
	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/rightsaudit"
)

func TestBuildClassifiesAndQueuesOnlyUnresolvedEvidence(t *testing.T) {
	recs := []*record.Record{
		auditRecord("exp-0004", "", ""),
		auditRecord("exp-0001", record.SourceLicenseProjectAuthored, ""),
		auditRecord("exp-0003", "Mystery-1.0", "https://example.test/mystery"),
		auditRecord("exp-0002", "MIT", ""),
		auditRecord("exp-0005", "GPL-3.0-only", "https://example.test/gpl"),
	}
	recs[3].Provenance.SourceAttribution = &record.SourceAttribution{CopyrightNotice: "Copyright Example", LicenseText: "MIT text"}

	got := rightsaudit.Build("/corpus", recs)
	if got.SchemaVersion != 1 || got.TotalRecords != 5 || got.CommercialEligible != 1 || got.UnresolvedEvidence != 3 {
		t.Fatalf("summary = %+v", got)
	}
	wantCodes := []string{
		pack.ReasonCopyleft,
		pack.ReasonMissingEvidence,
		pack.ReasonMissingSourceURL,
		pack.ReasonProjectAuthored,
		pack.ReasonUnrecognizedLicense,
	}
	if len(got.ReasonBuckets) != len(wantCodes) {
		t.Fatalf("reason buckets = %+v", got.ReasonBuckets)
	}
	for i, want := range wantCodes {
		if got.ReasonBuckets[i].Code != want || got.ReasonBuckets[i].Count != 1 {
			t.Errorf("bucket[%d] = %+v, want %s/1", i, got.ReasonBuckets[i], want)
		}
	}
	if len(got.RemediationQueue) != 3 {
		t.Fatalf("remediation queue = %+v", got.RemediationQueue)
	}
	for i, wantID := range []string{"exp-0002", "exp-0003", "exp-0004"} {
		item := got.RemediationQueue[i]
		if item.ID != wantID || item.AutomaticChange {
			t.Errorf("queue[%d] = %+v", i, item)
		}
		if item.Action == "" {
			t.Errorf("queue[%d] has no review action", i)
		}
	}
	if recs[0].Provenance.SourceLicense != "" {
		t.Fatal("audit must never assert rights or mutate records")
	}
}

func auditRecord(id, license, url string) *record.Record {
	r := &record.Record{
		ID: id, Path: "experience/2026/" + id[4:] + "-fixture.md", Status: "validated",
		Provenance: record.Provenance{SourceLicense: license, SourceURL: url},
	}
	r.Provenance.RightsReview = &record.RightsReview{
		Reviewer: "Jane Rights Reviewer", ReviewedAt: "2026-07-10T12:00:00Z",
		SourceSHA256: "sha256:" + strings.Repeat("a", 64), Policy: pack.RightsPolicyV1,
	}
	r.Provenance.RightsReview.EvidenceSHA256 = pack.EvidenceDigest(r)
	return r
}
