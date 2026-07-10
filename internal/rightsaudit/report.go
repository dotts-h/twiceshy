// SPDX-License-Identifier: AGPL-3.0-only

// Package rightsaudit builds deterministic, read-only commercial-rights audit
// reports and human-review remediation queues from experience records.
package rightsaudit

import (
	"sort"
	"strings"

	"github.com/dotts-h/twiceshy/internal/pack"
	"github.com/dotts-h/twiceshy/internal/record"
)

// ReasonBucket is one stable pack-eligibility reason and its record count.
type ReasonBucket struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

// Finding is the record-level audit result. It contains the evidence observed,
// never a guessed or synthesized rights assertion.
type Finding struct {
	ID                 string `json:"id"`
	Path               string `json:"path"`
	Status             string `json:"status"`
	SourceLicense      string `json:"source_license,omitempty"`
	SourceURL          string `json:"source_url,omitempty"`
	Commercial         bool   `json:"commercial_eligible"`
	NeedsNotice        bool   `json:"needs_notice"`
	ReasonCode         string `json:"reason_code"`
	Reason             string `json:"reason"`
	UnresolvedEvidence bool   `json:"unresolved_evidence"`
}

// Remediation is a machine-actionable human-review queue item. AutomaticChange
// is always false: the workflow never rewrites records or asserts ownership.
type Remediation struct {
	ID              string `json:"id"`
	Path            string `json:"path"`
	ReasonCode      string `json:"reason_code"`
	SourceLicense   string `json:"source_license,omitempty"`
	SourceURL       string `json:"source_url,omitempty"`
	Action          string `json:"action"`
	AutomaticChange bool   `json:"automatic_change"`
}

// ArtifactValidation records an optional pre-ship manifest/notice check.
type ArtifactValidation struct {
	Requested bool     `json:"requested"`
	Valid     bool     `json:"valid"`
	Errors    []string `json:"errors"`
}

// Report is the versioned deterministic rights-audit result.
type Report struct {
	SchemaVersion      int                 `json:"schema_version"`
	Corpus             string              `json:"corpus"`
	TotalRecords       int                 `json:"total_records"`
	CommercialEligible int                 `json:"commercial_eligible"`
	NeedsNotice        int                 `json:"needs_notice"`
	UnresolvedEvidence int                 `json:"unresolved_evidence"`
	ReasonBuckets      []ReasonBucket      `json:"reason_buckets"`
	Findings           []Finding           `json:"findings"`
	RemediationQueue   []Remediation       `json:"remediation_queue"`
	ArtifactValidation *ArtifactValidation `json:"artifact_validation,omitempty"`
}

// Build classifies every record through pack.ClassifyRecord. It is pure and
// read-only; callers own output persistence.
func Build(corpus string, recs []*record.Record) Report {
	rep := Report{SchemaVersion: 1, Corpus: corpus, TotalRecords: len(recs)}
	counts := make(map[string]int)
	for _, rec := range recs {
		if rec == nil {
			continue
		}
		e := pack.ClassifyRecord(rec)
		unresolved := unresolvedEvidence(e.Code)
		finding := Finding{
			ID: rec.ID, Path: rec.Path, Status: rec.Status,
			SourceLicense: strings.TrimSpace(rec.Provenance.SourceLicense),
			SourceURL:     strings.TrimSpace(rec.Provenance.SourceURL),
			Commercial:    e.Commercial, NeedsNotice: e.NeedsAttribution,
			ReasonCode: e.Code, Reason: e.Reason, UnresolvedEvidence: unresolved,
		}
		rep.Findings = append(rep.Findings, finding)
		counts[e.Code]++
		if e.Commercial {
			rep.CommercialEligible++
		}
		if e.Commercial && e.NeedsAttribution {
			rep.NeedsNotice++
		}
		if unresolved {
			rep.UnresolvedEvidence++
			rep.RemediationQueue = append(rep.RemediationQueue, Remediation{
				ID: rec.ID, Path: rec.Path, ReasonCode: e.Code,
				SourceLicense: finding.SourceLicense, SourceURL: finding.SourceURL,
				Action: remediationAction(e.Code), AutomaticChange: false,
			})
		}
	}
	for code, count := range counts {
		rep.ReasonBuckets = append(rep.ReasonBuckets, ReasonBucket{Code: code, Count: count})
	}
	sort.Slice(rep.ReasonBuckets, func(i, j int) bool { return rep.ReasonBuckets[i].Code < rep.ReasonBuckets[j].Code })
	sort.Slice(rep.Findings, func(i, j int) bool { return rep.Findings[i].ID < rep.Findings[j].ID })
	sort.Slice(rep.RemediationQueue, func(i, j int) bool { return rep.RemediationQueue[i].ID < rep.RemediationQueue[j].ID })
	return rep
}

func unresolvedEvidence(code string) bool {
	switch code {
	case pack.ReasonMissingEvidence, pack.ReasonMissingSourceURL, pack.ReasonMissingNoticeEvidence, pack.ReasonUnrecognizedLicense:
		return true
	default:
		return false
	}
}

func remediationAction(code string) string {
	switch code {
	case pack.ReasonMissingEvidence:
		return "review provenance and record truthful rights evidence in a human-reviewed corpus PR"
	case pack.ReasonMissingSourceURL:
		return "verify the licensed source and record its immutable source URL in a human-reviewed corpus PR"
	case pack.ReasonMissingNoticeEvidence:
		return "collect complete source attribution, notice, and license text evidence in a human-reviewed corpus PR"
	case pack.ReasonUnrecognizedLicense:
		return "verify the source terms and record a supported SPDX license or keep the record excluded"
	default:
		return "manual rights review required; keep the record excluded until resolved"
	}
}
