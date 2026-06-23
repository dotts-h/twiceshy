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

const defaultNpmBase = "https://registry.npmjs.org"

// maxNpmRespBytes caps the per-package response we decode. We hit the lightweight
// `/<pkg>/latest` endpoint (one version object), so this is generous.
const maxNpmRespBytes = 1 << 18 // 256 KiB

// defaultNpmPackages is the seed set of npm packages the watcher checks for a
// deprecation flag — common packages an agent might still reach for, several of
// which are deprecated with a documented replacement. Curated and small; widen as
// the corpus needs coverage. An unknown package 404s and is skipped.
var defaultNpmPackages = []string{
	"request", "request-promise", "node-sass", "tslint", "babel-eslint",
	"gulp-util", "istanbul", "left-pad", "core-js", "har-validator",
}

// NpmLiveSource watches the npm registry for packages whose latest version carries
// a deprecation flag and emits license-clean, quarantined deprecation Drafts — a
// deprecated dependency is a trap an agent should avoid. Only the non-copyrightable
// FACT (this package's latest version is deprecated) is used; the maintainer's
// deprecation message is never reproduced — all prose is generated in twiceshy's own
// words (ADR-0003 §4). The non-OSV web watcher of #0073, adjacent to the endoflife
// importer (#0023).
type NpmLiveSource struct {
	packages []string
	fetch    func(ctx context.Context, pkg string) (io.ReadCloser, error)
}

// NpmLiveOption configures an NpmLiveSource.
type NpmLiveOption func(*NpmLiveSource)

// WithNpmFetch overrides the per-package fetcher (tests inject a fixture; production
// hits the registry). A nil reader means the package was not found (a 404), skipped.
func WithNpmFetch(fetch func(ctx context.Context, pkg string) (io.ReadCloser, error)) NpmLiveOption {
	return func(s *NpmLiveSource) { s.fetch = fetch }
}

// WithNpmPackages sets the packages to check (empty is ignored).
func WithNpmPackages(pkgs []string) NpmLiveOption {
	return func(s *NpmLiveSource) {
		if len(pkgs) > 0 {
			s.packages = pkgs
		}
	}
}

// NewNpmLiveSource returns a live npm-deprecation watcher over the default package set.
func NewNpmLiveSource(opts ...NpmLiveOption) Source {
	s := &NpmLiveSource{packages: defaultNpmPackages}
	for _, opt := range opts {
		opt(s)
	}
	if s.fetch == nil {
		s.fetch = npmLiveFetcher(defaultNpmBase)
	}
	return s
}

// npmLiveFetcher returns the production per-package fetcher for `<base>/<pkg>/latest`.
// A 404 (unknown/unpublished package) is reported as a nil reader so the import skips
// it rather than failing the batch.
func npmLiveFetcher(base string) func(context.Context, string) (io.ReadCloser, error) {
	return func(ctx context.Context, pkg string) (io.ReadCloser, error) {
		url := fmt.Sprintf("%s/%s/latest", base, pkg)
		client := &http.Client{Timeout: 30 * time.Second}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("npm-deprecation: build request for %s: %w", pkg, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("npm-deprecation: fetch %s: %w", pkg, err)
		}
		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return nil, nil // unknown package — skip, not a failure
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("npm-deprecation: fetch %s: HTTP %d", url, resp.StatusCode)
		}
		return resp.Body, nil
	}
}

func (s *NpmLiveSource) Name() string { return "npm-deprecation" }

// npmVersionMeta is the subset of npm's `/<pkg>/latest` we read. `deprecated` is
// either a string (the maintainer's notice — we read only that it is present, not
// its text) or a bool; RawMessage lets isDeprecated decide without binding the shape.
type npmVersionMeta struct {
	Version    string          `json:"version"`
	Deprecated json.RawMessage `json:"deprecated"`
}

// isDeprecated reports whether npm's `deprecated` field marks the version deprecated:
// any non-empty string, or the literal `true`. Absent / null / false / "" do not.
func isDeprecated(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	switch s {
	case "", "null", "false", `""`:
		return false
	default:
		return true
	}
}

// Drafts checks each seed package's latest version for a deprecation flag and maps
// the deprecated ones to quarantined drafts, sorted by signature for deterministic
// output. A malformed body for one package is skipped (skip-junk); a fetch/HTTP
// error or a cancelled context is systemic and fails the batch.
func (s *NpmLiveSource) Drafts(ctx context.Context) ([]Draft, error) {
	var drafts []Draft
	for _, pkg := range s.packages {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		rc, err := s.fetch(ctx, pkg)
		if err != nil {
			return nil, err
		}
		if rc == nil {
			continue // unknown package (404)
		}
		var meta npmVersionMeta
		decErr := json.NewDecoder(io.LimitReader(rc, maxNpmRespBytes)).Decode(&meta)
		_ = rc.Close()
		if decErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err() // systemic (cancelled mid-decode) — fail loud
			}
			continue // a malformed 200 body for one package must not zero out the rest
		}
		if meta.Version == "" || !isDeprecated(meta.Deprecated) {
			continue
		}
		drafts = append(drafts, npmDraft(pkg, meta.Version))
	}
	sort.Slice(drafts, func(i, j int) bool {
		return drafts[i].Symptom.ErrorSignatures[0] < drafts[j].Symptom.ErrorSignatures[0]
	})
	return drafts, nil
}

// npmDraft maps a deprecated package to a deprecation draft. The deprecated state
// and the version are facts (verbatim); the title/summary/fix prose is generated and
// points at the npm page for the maintainer's notice + replacement (never reproduced).
func npmDraft(pkg, version string) Draft {
	sig := fmt.Sprintf("npm warn deprecated %s@%s", pkg, version)
	url := fmt.Sprintf("https://www.npmjs.com/package/%s", pkg)
	return Draft{
		Kind:  "fix",
		Title: fmt.Sprintf("npm package %s is deprecated", pkg),
		Symptom: &record.Symptom{
			Summary:         fmt.Sprintf("npm marks %s (latest %s) as deprecated — installing it warns and it no longer receives fixes.", pkg, version),
			ErrorSignatures: []string{sig},
		},
		AppliesTo: []record.AppliesTo{{Ecosystem: "npm", Package: pkg}},
		Resolution: &record.Resolution{
			RootCause: fmt.Sprintf("The maintainer published an npm deprecation notice for %s; deprecated packages stop receiving fixes and signal a migration.", pkg),
			Fix:       fmt.Sprintf("Stop depending on %s; see %s for the maintainer's deprecation notice and recommended replacement.", pkg, url),
		},
		Body:          fmt.Sprintf("The npm registry marks %s (latest %s) as deprecated. Code depending on it should migrate to the maintainer's recommended replacement; see %s.", pkg, version, url),
		SourceLicense: record.SourceLicenseFactsOnly,
		SourceURL:     url,
	}
}
