// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
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

const defaultEOLBase = "https://endoflife.date"

// maxEOLRespBytes caps an endoflife.date response body we will decode — the base
// URL is operator-configurable, so bound what a misbehaving source can stream.
const maxEOLRespBytes = 1 << 20

// defaultEOLProducts is the seed set of endoflife.date products the live importer
// scans (the common runtimes agent code targets). Kept small and curated; widen as the
// corpus needs coverage. An unknown product 404s and is skipped, so an over-broad list
// is harmless.
var defaultEOLProducts = []string{"python", "nodejs", "go", "ruby", "php", "django", "java", "dotnet"}

// EOLLiveSource fetches release-cycle end-of-life data from endoflife.date (MIT data)
// and emits license-clean, quarantined deprecation Drafts: a runtime cycle past its
// end-of-life date is a deprecation an agent should avoid. Only non-copyrightable facts
// (product, cycle, EOL date) are used verbatim; all prose is generated in twiceshy's own
// words (ADR-0003 §4). It is the deps.dev/endoflife half of ADR-0011's live feed (#0023),
// adjacent to the live OSV importer (#0021).
type EOLLiveSource struct {
	products []string
	now      time.Time
	fetch    func(ctx context.Context, product string) (io.ReadCloser, error)
}

// EOLLiveOption configures an EOLLiveSource.
type EOLLiveOption func(*EOLLiveSource)

// WithEOLLiveFetch overrides the per-product fetcher (tests inject a fixture;
// production hits endoflife.date). The fetcher returns a nil reader for an unknown
// product (a 404), which the importer skips.
func WithEOLLiveFetch(fetch func(ctx context.Context, product string) (io.ReadCloser, error)) EOLLiveOption {
	return func(s *EOLLiveSource) { s.fetch = fetch }
}

// WithEOLProducts sets the endoflife.date products to scan (empty is ignored).
func WithEOLProducts(products []string) EOLLiveOption {
	return func(s *EOLLiveSource) {
		if len(products) > 0 {
			s.products = products
		}
	}
}

// WithEOLNow pins the clock used to decide whether a cycle's EOL date has passed (tests
// inject a fixed time for deterministic output).
func WithEOLNow(now time.Time) EOLLiveOption {
	return func(s *EOLLiveSource) { s.now = now }
}

// NewEOLLiveSource returns a live endoflife.date importer over the default product set.
func NewEOLLiveSource(opts ...EOLLiveOption) Source {
	s := &EOLLiveSource{products: defaultEOLProducts, now: time.Now().UTC()}
	for _, opt := range opts {
		opt(s)
	}
	if s.fetch == nil {
		s.fetch = eolLiveFetcher(defaultEOLBase)
	}
	return s
}

// eolLiveFetcher returns the production per-product fetcher. A 404 (unknown product) is
// reported as a nil reader so the import skips it rather than failing the batch.
func eolLiveFetcher(base string) func(context.Context, string) (io.ReadCloser, error) {
	return func(ctx context.Context, product string) (io.ReadCloser, error) {
		url := fmt.Sprintf("%s/api/%s.json", base, product)
		client := &http.Client{Timeout: 30 * time.Second}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("eol-live: build request for %s: %w", product, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("eol-live: fetch %s: %w", product, err)
		}
		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return nil, nil // unknown product — skip, not a failure
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("eol-live: fetch %s: HTTP %d", url, resp.StatusCode)
		}
		return resp.Body, nil
	}
}

func (s *EOLLiveSource) Name() string { return "eol-live" }

// Drafts fetches each product's release cycles and maps every already-end-of-life cycle
// to a quarantined deprecation draft, sorted by signature for deterministic output.
func (s *EOLLiveSource) Drafts(ctx context.Context) ([]Draft, error) {
	var drafts []Draft
	for _, product := range s.products {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		rc, err := s.fetch(ctx, product)
		if err != nil {
			return nil, err
		}
		if rc == nil {
			continue // unknown product (404)
		}
		var cycles []eolCycle
		decErr := json.NewDecoder(io.LimitReader(rc, maxEOLRespBytes)).Decode(&cycles)
		_ = rc.Close()
		if decErr != nil {
			// A cancelled/deadline-exceeded run mid-decode is systemic — fail loud
			// rather than bury it as a per-product malformed-body skip.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// A malformed 200 body for ONE product must not zero out the others — skip it
			// and continue (the bulk-importer "skip junk" principle; a later scheduled run
			// retries). A fetch/HTTP error above is systemic, so it still fails loud.
			continue
		}
		for _, c := range cycles {
			if d, ok := eolDraft(product, c, s.now); ok {
				drafts = append(drafts, d)
			}
		}
	}
	sort.Slice(drafts, func(i, j int) bool {
		return drafts[i].Symptom.ErrorSignatures[0] < drafts[j].Symptom.ErrorSignatures[0]
	})
	return drafts, nil
}

// eolCycle is one endoflife.date release cycle (only the fields the importer reads).
type eolCycle struct {
	Cycle string   `json:"cycle"`
	EOL   eolField `json:"eol"`
}

// eolField decodes endoflife.date's `eol`, which is either a date string ("EOL on this
// date", possibly future) or a bool (true = already EOL with no date, false = not EOL).
type eolField struct {
	date      string
	alreadyOL bool
}

func (e *eolField) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		e.date = strings.TrimSpace(s)
		return nil
	}
	var ok bool
	if err := json.Unmarshal(b, &ok); err == nil {
		e.alreadyOL = ok
		return nil
	}
	return fmt.Errorf("eol field is neither a date string nor a bool: %s", b)
}

// eolDraft maps a release cycle to a deprecation draft, returning ok=false for a cycle
// that is not yet end-of-life (a future EOL date, or eol:false). The EOL date is a fact
// (verbatim); the title/summary/fix prose is generated.
func eolDraft(product string, c eolCycle, now time.Time) (Draft, bool) {
	cycle := strings.TrimSpace(c.Cycle)
	if cycle == "" {
		return Draft{}, false
	}
	dateNote := "date unspecified"
	switch {
	case c.EOL.date != "":
		t, err := time.Parse("2006-01-02", c.EOL.date)
		if err != nil {
			// An unparseable EOL date can't be placed before/after now — skip it rather
			// than emit a record carrying the garbage token (the bulk-importer "skip
			// junk" principle, like the OSV importer).
			return Draft{}, false
		}
		// A future EOL date is not yet end-of-life — skip until it passes (a later run
		// picks it up, born quarantined; dedup keeps re-imports idempotent).
		if t.After(now) {
			return Draft{}, false
		}
		dateNote = c.EOL.date
	case c.EOL.alreadyOL:
		// already end-of-life, date unknown — keep dateNote = "date unspecified"
	default:
		return Draft{}, false // eol:false — supported
	}

	sig := fmt.Sprintf("EOL:%s:%s", product, cycle)
	url := fmt.Sprintf("https://endoflife.date/%s", product)
	return Draft{
		Kind:  "fix",
		Title: fmt.Sprintf("%s %s is end-of-life", product, cycle),
		Symptom: &record.Symptom{
			Summary:         fmt.Sprintf("%s %s reached end-of-life (%s) — it no longer receives security or maintenance updates.", product, cycle, dateNote),
			ErrorSignatures: []string{sig},
		},
		AppliesTo: []record.AppliesTo{{Runtime: map[string]string{product: cycle}}},
		Resolution: &record.Resolution{
			RootCause: fmt.Sprintf("The %s %s release cycle is past its end-of-life (%s); end-of-life runtimes stop receiving security patches.", product, cycle, dateNote),
			Fix:       fmt.Sprintf("Upgrade to a currently-supported %s release; see %s for the support timeline.", product, url),
		},
		Body:          fmt.Sprintf("endoflife.date lists %s %s as end-of-life (%s). Code still targeting it should move to a supported release. See %s.", product, cycle, dateNote, url),
		SourceLicense: record.SourceLicenseFactsOnly,
		SourceURL:     url,
	}, true
}
