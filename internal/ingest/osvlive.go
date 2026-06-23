// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
)

const (
	osvLiveExportBase       = "https://osv-vulnerabilities.storage.googleapis.com"
	defaultOSVLiveEcosystem = "Go"
)

// OSVLiveSource fetches live advisories for ONE OSV ecosystem from osv.dev's bulk
// export and emits license-clean, quarantined-record Drafts. Functional identifiers
// (GO/CVE/GHSA ids, package names, version ranges) are verbatim; all prose is
// generated in twiceshy's own words (ADR-0003 §4, ADR-0011 §5/§8). The ecosystem is
// configurable (WithEcosystem) so the corpus can cover a whole stack — npm for
// React/React Native, PyPI for Python, Go — by running the importer once per
// ecosystem.
type OSVLiveSource struct {
	ecosystem string
	fetch     func(context.Context) (io.ReadCloser, error)
}

// OSVLiveOption configures an OSVLiveSource.
type OSVLiveOption func(*OSVLiveSource)

// WithOSVLiveFetch overrides the zip fetcher (tests inject a fixture; production
// uses the default osv.dev per-ecosystem bulk export).
func WithOSVLiveFetch(fetch func(context.Context) (io.ReadCloser, error)) OSVLiveOption {
	return func(s *OSVLiveSource) {
		s.fetch = fetch
	}
}

// WithEcosystem sets which OSV ecosystem to import — the exact OSV ecosystem label,
// which is also the bulk-export path segment (e.g. "npm", "PyPI", "Go"). Default
// "Go"; an empty value is ignored (keeps the default).
func WithEcosystem(ecosystem string) OSVLiveOption {
	return func(s *OSVLiveSource) {
		if e := strings.TrimSpace(ecosystem); e != "" {
			s.ecosystem = e
		}
	}
}

// NewOSVLiveSource returns a live OSV importer (default ecosystem Go). The fetcher
// is built from the FINAL ecosystem after options apply, unless a test injected one.
func NewOSVLiveSource(opts ...OSVLiveOption) Source {
	s := &OSVLiveSource{ecosystem: defaultOSVLiveEcosystem}
	for _, opt := range opts {
		opt(s)
	}
	if s.fetch == nil {
		s.fetch = osvLiveFetcher(s.ecosystem)
	}
	return s
}

// osvLiveFetcher returns the production fetcher for one ecosystem's bulk export.
func osvLiveFetcher(ecosystem string) func(context.Context) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/%s/all.zip", osvLiveExportBase, ecosystem)
	return func(ctx context.Context) (io.ReadCloser, error) {
		client := &http.Client{Timeout: 2 * time.Minute}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("osv-live: build request: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("osv-live: fetch export: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("osv-live: fetch %s: HTTP %d", url, resp.StatusCode)
		}
		return resp.Body, nil
	}
}

func (s *OSVLiveSource) Name() string { return "osv-live" }

// Drafts fetches the Go bulk export, maps each OSV record to a trap draft, and
// returns them sorted by advisory id for deterministic output.
func (s *OSVLiveSource) Drafts(ctx context.Context) ([]Draft, error) {
	rc, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("osv-live: read export: %w", err)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("osv-live: open zip: %w", err)
	}
	return draftsFromZip(zr, s.ecosystem)
}

func draftsFromZip(zr *zip.Reader, ecosystem string) ([]Draft, error) {
	var drafts []Draft
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.HasSuffix(f.Name, ".json") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("osv-live: open zip entry %q: %w", f.Name, err)
		}
		var rec osvLiveRecord
		if err := json.NewDecoder(rc).Decode(&rec); err != nil {
			_ = rc.Close()
			return nil, fmt.Errorf("osv-live: decode %q: %w", f.Name, err)
		}
		_ = rc.Close()
		if d, ok := mapOSVLiveRecord(rec, ecosystem); ok {
			drafts = append(drafts, d)
		}
	}
	sort.Slice(drafts, func(i, j int) bool {
		return drafts[i].Symptom.ErrorSignatures[0] < drafts[j].Symptom.ErrorSignatures[0]
	})
	return drafts, nil
}

type osvLiveRecord struct {
	ID         string            `json:"id"`
	Aliases    []string          `json:"aliases"`
	Summary    string            `json:"summary"`
	Details    string            `json:"details"`
	Withdrawn  string            `json:"withdrawn"`
	Affected   []osvLiveAffected `json:"affected"`
	References []osvLiveRef      `json:"references"`
}

type osvLiveAffected struct {
	Package osvLivePackage `json:"package"`
	Ranges  []osvLiveRange `json:"ranges"`
}

type osvLivePackage struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
}

type osvLiveRange struct {
	Events []osvLiveEvent `json:"events"`
}

type osvLiveEvent struct {
	Introduced string `json:"introduced"`
	Fixed      string `json:"fixed"`
}

type osvLiveRef struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func mapOSVLiveRecord(rec osvLiveRecord, ecosystem string) (Draft, bool) {
	if strings.TrimSpace(rec.Withdrawn) != "" {
		return Draft{}, false
	}
	// An advisory with no id can't carry a usable error_signature (an empty sig
	// fails validateSymptom in Prepare and would abort the whole import batch), so
	// skip it like a withdrawn entry — a bulk importer skips junk, never fails.
	if strings.TrimSpace(rec.ID) == "" {
		return Draft{}, false
	}
	var applies []record.AppliesTo
	primaryPkg := ""
	for _, aff := range rec.Affected {
		if aff.Package.Ecosystem != ecosystem {
			continue
		}
		if strings.TrimSpace(aff.Package.Name) == "" {
			continue // a vuln "in <nothing>" is not actionable; skip nameless affected blocks
		}
		if primaryPkg == "" {
			primaryPkg = aff.Package.Name
		}
		for _, r := range aff.Ranges {
			pairs := osvLiveRangePairs(r.Events)
			if len(pairs) == 0 {
				// No events → preserve the prior "whole package" mapping (nil range).
				pairs = []versionInterval{{}}
			}
			for _, iv := range pairs {
				applies = append(applies, record.AppliesTo{
					Ecosystem: ecosystem,
					Package:   aff.Package.Name,
					Versions:  versionRange(iv.introduced, iv.fixed),
				})
			}
		}
	}
	if len(applies) == 0 {
		return Draft{}, false
	}

	sigs := make([]string, 0, 1+len(rec.Aliases))
	sigs = append(sigs, rec.ID)
	sigs = append(sigs, rec.Aliases...)

	ids := rec.ID
	if len(rec.Aliases) > 0 {
		ids = fmt.Sprintf("%s (%s)", rec.ID, strings.Join(rec.Aliases, ", "))
	}
	summary := fmt.Sprintf("%s: known vulnerability in %s", ids, primaryPkg)
	sourceURL := osvLiveGHSAURL(rec.References)
	if sourceURL == "" {
		sourceURL = fmt.Sprintf("https://osv.dev/vulnerability/%s", rec.ID)
	}
	title := fmt.Sprintf("%s: vulnerability in %s", rec.ID, primaryPkg)
	body := osvLiveBody(rec.ID, applies, sourceURL)
	rootCause := fmt.Sprintf("Known vulnerability documented in OSV advisory %s.", rec.ID)
	fix := osvLiveFixText(applies, sourceURL)

	return buildOSVDraft(osvDraftInput{
		Signatures: sigs,
		AppliesTo:  applies,
		Title:      title,
		Summary:    summary,
		RootCause:  rootCause,
		Fix:        fix,
		Body:       body,
		SourceURL:  sourceURL,
	}), true
}

// osvLiveFixText renders the remediation. If the advisory publishes a fixed
// version (any affected range carries one) it advises upgrading past it; if none
// is published (fixed:null) it must NOT claim a fixed version exists — that
// "upgrade past the fixed version" boilerplate contradicts the record and is the
// largest #0061 transcription-defect class (#0062 pairs by making the judge see it).
func osvLiveFixText(applies []record.AppliesTo, sourceURL string) string {
	for _, a := range applies {
		if a.Versions != nil && a.Versions.Fixed != nil {
			return fmt.Sprintf("Upgrade affected packages past the fixed version; see %s.", sourceURL)
		}
	}
	return fmt.Sprintf("No fix is published yet (the advisory lists no fixed version); see %s for status and mitigations.", sourceURL)
}

// versionInterval is one (introduced, fixed) affected range; fixed=="" is open-ended.
type versionInterval struct{ introduced, fixed string }

// osvLiveRangePairs walks a range's ordered events and pairs each `introduced` with
// the next `fixed`, so disjoint affected intervals stay separate instead of
// collapsing to first-introduced/last-fixed (which would falsely claim the gap
// between them is affected). A trailing open `introduced` closes as fixed:null.
func osvLiveRangePairs(events []osvLiveEvent) []versionInterval {
	var pairs []versionInterval
	var cur versionInterval
	open := false
	for _, e := range events {
		if e.Introduced != "" {
			if open {
				// a new introduced with no intervening fixed closes the prior open-ended
				pairs = append(pairs, cur)
			}
			cur = versionInterval{introduced: e.Introduced}
			open = true
		}
		if e.Fixed != "" {
			cur.fixed = e.Fixed
			pairs = append(pairs, cur)
			cur = versionInterval{}
			open = false
		}
	}
	if open {
		pairs = append(pairs, cur)
	}
	return pairs
}

func osvLiveGHSAURL(refs []osvLiveRef) string {
	for _, r := range refs {
		if strings.Contains(r.URL, "GHSA-") {
			return r.URL
		}
	}
	return ""
}

func osvLiveBody(id string, applies []record.AppliesTo, sourceURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "OSV advisory %s affects ", id)
	for i, a := range applies {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s", a.Package)
		if a.Versions != nil {
			if a.Versions.Introduced != nil {
				fmt.Fprintf(&b, " (introduced %s", *a.Versions.Introduced)
				if a.Versions.Fixed != nil {
					fmt.Fprintf(&b, ", fixed %s", *a.Versions.Fixed)
				}
				b.WriteString(")")
			} else if a.Versions.Fixed != nil {
				fmt.Fprintf(&b, " (fixed %s)", *a.Versions.Fixed)
			}
		}
	}
	fmt.Fprintf(&b, ". See %s.", sourceURL)
	return b.String()
}
