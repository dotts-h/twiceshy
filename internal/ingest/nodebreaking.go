// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
)

// nodeChangelogURLTemplate is the raw Node.js changelog file for one major line
// (e.g. "V22" -> CHANGELOG_V22.md), which lists every release of that major.
const nodeChangelogURLTemplate = "https://raw.githubusercontent.com/nodejs/node/main/doc/changelogs/CHANGELOG_%s.md"

// maxNodeChangelogRespBytes caps the changelog body we read — a full major-line
// file runs under 1 MiB today, so this is generous headroom.
const maxNodeChangelogRespBytes = 1 << 22 // 4 MiB

// MaxNodeBreakingDrafts defensively caps the drafts NodeBreakingSource emits in one
// Drafts() call — a Node changelog file accumulates a SEMVER-MAJOR entry per
// release across its whole major line, so this is a backstop against unbounded
// growth, not the primary gate: `twiceshy ingest -limit` at import time is.
const MaxNodeBreakingDrafts = 200

// defaultNodeTargets is the curated set of Node major-line changelog files the
// watcher mines: current + active-LTS majors. Bump this per new Node major
// release/LTS promotion; an unknown/retired major 404s and is skipped.
var defaultNodeTargets = []string{"V24", "V22"}

// nodeVersionHeadingRe matches a changelog release-section heading, e.g.
// "## 2026-06-18, Version 22.23.0 'Jod' (LTS), @aduh95", capturing the version.
var nodeVersionHeadingRe = regexp.MustCompile(`^## \d{4}-\d{2}-\d{2}, Version (\S+)`)

// nodeSemverMajorRe matches a Node changelog commit line flagged (SEMVER-MAJOR),
// e.g.:
//
//   - \[[`ea5dc6b529`](https://github.com/nodejs/node/commit/ea5dc6b529)] - **(SEMVER-MAJOR)** **http2**: remove support for priority signaling (Matteo Collina) [#58293](https://github.com/nodejs/node/pull/58293)
//
// capturing subsystem, subject, and the PR URL. Subject may itself contain
// parenthesized text (e.g. "console.assert()"); the author-parens group requires
// at least one non-paren rune, so an empty "()" in the subject is never mistaken
// for the trailing "(author)" group. A line whose shape doesn't match (e.g. the
// rare "_**Revert**_ "**subsystem**: ..."" form) is skipped, not fatal.
var nodeSemverMajorRe = regexp.MustCompile(`\*\*\(SEMVER-MAJOR\)\*\*\s*\*\*([^*]+)\*\*:\s*(.+?)\s*\(([^()]+)\)\s*\[#\d+\]\(([^)]+)\)\s*$`)

// NodeBreakingSource watches Node.js's own changelogs for SEMVER-MAJOR-flagged
// commits and emits license-clean, quarantined trap Drafts: a SEMVER-MAJOR entry
// is a documented breaking change, and a breaking change shipped after a model's
// training cutoff is exactly the kind of thing it gets systematically wrong. Only
// the factual subsystem/subject naming the change is used; no changelog prose
// beyond that short factual line is reproduced (ADR-0003 §4). MIT, Node-authored
// changelog text only (WEB_SOURCES.md row 14).
type NodeBreakingSource struct {
	targets []string
	fetch   func(ctx context.Context, target string) (io.ReadCloser, error)
}

// NodeBreakingOption configures a NodeBreakingSource.
type NodeBreakingOption func(*NodeBreakingSource)

// WithNodeBreakingFetch overrides the per-target fetcher (tests inject a fixture;
// production hits raw.githubusercontent.com). A nil reader means the major line
// was not found (a 404), skipped.
func WithNodeBreakingFetch(fetch func(ctx context.Context, target string) (io.ReadCloser, error)) NodeBreakingOption {
	return func(s *NodeBreakingSource) { s.fetch = fetch }
}

// WithNodeBreakingTargets sets the changelog major-line targets to fetch (e.g.
// "V22"); empty is ignored.
func WithNodeBreakingTargets(targets []string) NodeBreakingOption {
	return func(s *NodeBreakingSource) {
		if len(targets) > 0 {
			s.targets = targets
		}
	}
}

// NewNodeBreakingSource returns a live Node.js SEMVER-MAJOR changelog watcher over
// the default major-line target set.
func NewNodeBreakingSource(opts ...NodeBreakingOption) Source {
	s := &NodeBreakingSource{targets: defaultNodeTargets}
	for _, opt := range opts {
		opt(s)
	}
	if s.fetch == nil {
		s.fetch = nodeBreakingFetcher()
	}
	return s
}

// nodeBreakingFetcher returns the production per-target fetcher for a changelog
// file. A 404 (unknown/retired major) is reported as a nil reader so the import
// skips it rather than failing the batch.
func nodeBreakingFetcher() func(context.Context, string) (io.ReadCloser, error) {
	return func(ctx context.Context, target string) (io.ReadCloser, error) {
		url := fmt.Sprintf(nodeChangelogURLTemplate, target)
		client := &http.Client{Timeout: 30 * time.Second}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("node-breaking: build request for %s: %w", target, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("node-breaking: fetch %s: %w", target, err)
		}
		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return nil, nil // unknown/retired major — skip, not a failure
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("node-breaking: fetch %s: HTTP %d", url, resp.StatusCode)
		}
		return resp.Body, nil
	}
}

func (s *NodeBreakingSource) Name() string { return "node-breaking" }

// Drafts fetches each target major line's changelog and maps every SEMVER-MAJOR
// commit line to a quarantined trap draft, sorted by signature (version, then
// subsystem) and capped at MaxNodeBreakingDrafts for deterministic, bounded output.
func (s *NodeBreakingSource) Drafts(ctx context.Context) ([]Draft, error) {
	var drafts []Draft
	for _, target := range s.targets {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		rc, err := s.fetch(ctx, target)
		if err != nil {
			return nil, err
		}
		if rc == nil {
			continue // unknown/retired major (404)
		}
		body, readErr := io.ReadAll(io.LimitReader(rc, maxNodeChangelogRespBytes))
		_ = rc.Close()
		if readErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err() // systemic (cancelled mid-read) — fail loud
			}
			continue // a broken body for one target must not zero out the rest
		}
		url := fmt.Sprintf(nodeChangelogURLTemplate, target)
		drafts = append(drafts, parseNodeChangelog(string(body), url)...)
	}
	sort.Slice(drafts, func(i, j int) bool {
		si, sj := drafts[i].Symptom.ErrorSignatures[0], drafts[j].Symptom.ErrorSignatures[0]
		if si != sj {
			return si < sj
		}
		return drafts[i].Title < drafts[j].Title // deterministic tie-break within one version+subsystem
	})
	if len(drafts) > MaxNodeBreakingDrafts {
		drafts = drafts[:MaxNodeBreakingDrafts]
	}
	return drafts, nil
}

// parseNodeChangelog scans one changelog file's Markdown for SEMVER-MAJOR commit
// lines, tracking the enclosing release-section version from the nearest
// preceding "## <date>, Version <version>..." heading. A SEMVER-MAJOR line that
// doesn't match the expected shape is skipped (malformed, never fatal); a
// SEMVER-MAJOR line seen before any version heading is also skipped (no version
// to attribute it to).
func parseNodeChangelog(body, changelogURL string) []Draft {
	var drafts []Draft
	var version string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := nodeVersionHeadingRe.FindStringSubmatch(line); m != nil {
			version = m[1]
			continue
		}
		if version == "" || !strings.Contains(line, "(SEMVER-MAJOR)") {
			continue
		}
		m := nodeSemverMajorRe.FindStringSubmatch(line)
		if m == nil {
			continue // malformed SEMVER-MAJOR line — skip, never fatal
		}
		subsystem, subject, prURL := m[1], strings.TrimSpace(m[2]), m[4]
		drafts = append(drafts, nodeBreakingDraft(version, subsystem, subject, prURL, changelogURL))
	}
	return drafts
}

// nodeBreakingDraft maps one SEMVER-MAJOR changelog entry to a breaking-change
// trap draft. Version/subsystem/subject/PR URL are facts (verbatim, short); the
// title/summary/root-cause/fix prose is generated, never copied changelog prose.
func nodeBreakingDraft(version, subsystem, subject, prURL, changelogURL string) Draft {
	sig := fmt.Sprintf("node:breaking:%s:%s", version, subsystem)
	sourceURL := changelogURL
	if version != "" {
		// The anchor id Node's changelog carries right above each release heading
		// (e.g. <a id="22.23.0"></a>) is exactly the version string.
		sourceURL = fmt.Sprintf("%s#%s", changelogURL, version)
	}
	return Draft{
		Kind:  "trap",
		Title: fmt.Sprintf("Node.js %s: %s: %s is a breaking change", version, subsystem, subject),
		Symptom: &record.Symptom{
			Summary:         fmt.Sprintf("Code relying on the previous behavior breaks after upgrading to Node.js %s: %s marks %s as a SEMVER-MAJOR (breaking) change.", version, subsystem, subject),
			ErrorSignatures: []string{sig},
		},
		AppliesTo: []record.AppliesTo{{Runtime: map[string]string{"node": version}}},
		Resolution: &record.Resolution{
			RootCause: fmt.Sprintf("Node.js %s shipped a SEMVER-MAJOR change in %s: %s.", version, subsystem, subject),
			Fix:       fmt.Sprintf("Review the linked PR for the new behavior and migrate call sites accordingly: %s", prURL),
		},
		Body:          fmt.Sprintf("Node.js %s's changelog marks %s: %s (SEMVER-MAJOR). Review the linked PR (%s) and the changelog entry (%s) and migrate call sites accordingly.", version, subsystem, subject, prURL, sourceURL),
		SourceLicense: record.SourceLicenseFactsOnly,
		SourceURL:     sourceURL,
	}
}
