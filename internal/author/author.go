// SPDX-License-Identifier: AGPL-3.0-only

// Package author pre-stages a §5-clean authored record plus its repro skeletons —
// the file generation behind the `twiceshy author` convenience command (#0091).
// Authoring works without it (docs/AUTHORING.md); this turns it into a
// fill-in-the-blanks flow and pre-fills the authored-internal provenance so the
// licensing discipline (ADR-0011 §5: no source_url, authored-internal sentinel) is
// right by construction. Pure: Scaffold returns the files to write; the caller
// owns the disk and the no-overwrite policy.
package author

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
)

// Params drives a scaffold. ID is exp-NNNN, Slug is the kebab filename slug, Kind
// defaults to "trap", Author fills provenance.source.author. WithNegative adds a
// negative repro skeleton (for a documented dead-end).
type Params struct {
	ID           string
	Slug         string
	Title        string
	Kind         string
	Author       string
	WithNegative bool
}

// File is one scaffolded file: a corpus-relative path and its content.
type File struct {
	Path    string
	Content string
}

var (
	reID   = regexp.MustCompile(`^exp-[0-9]{4,}$`)
	reSlug = regexp.MustCompile(`^[a-z0-9-]+$`)
)

// Scaffold returns the record + repro skeleton files for an authored record. It
// validates the parameters and stamps now into recorded_at / valid.from; the
// record parses as a valid quarantined, §5-clean draft so the author can fill it
// in and revalidate immediately. Pure; no I/O.
func Scaffold(p Params, now time.Time) ([]File, error) {
	if !reID.MatchString(p.ID) {
		return nil, fmt.Errorf("author: id %q must look like exp-0091", p.ID)
	}
	if !reSlug.MatchString(p.Slug) {
		return nil, fmt.Errorf("author: slug %q must be kebab-case [a-z0-9-]", p.Slug)
	}
	// Match the record's title rule (record.go) so the skeleton always parses.
	if n := len([]rune(strings.TrimSpace(p.Title))); n < 8 || n > 120 {
		return nil, fmt.Errorf("author: title length %d is outside 8..120", n)
	}
	if strings.TrimSpace(p.Author) == "" {
		return nil, fmt.Errorf("author: author is required (provenance.source.author)")
	}
	kind := p.Kind
	if kind == "" {
		kind = "trap"
	}
	if !contains(record.Kinds, kind) {
		return nil, fmt.Errorf("author: kind %q is not one of %v", kind, record.Kinds)
	}

	num := strings.TrimPrefix(p.ID, "exp-")
	date := now.Format("2006-01-02")
	recordPath := fmt.Sprintf("experience/%d/%s-%s.md", now.Year(), num, p.Slug)
	positivePath := fmt.Sprintf("experience/repro/%s-%s.sh", num, p.Slug)
	negativePath := fmt.Sprintf("experience/repro/%s-%s-negative.sh", num, p.Slug)

	repros := "    - path: " + positivePath + "\n" +
		"      kind: positive\n" +
		"      label: \"TODO: what the positive repro proves (trap reproduced + escape works)\"\n"
	files := []File{
		{Path: positivePath, Content: positiveRepro(p.ID)},
	}
	if p.WithNegative {
		repros += "    - path: " + negativePath + "\n" +
			"      kind: negative\n" +
			"      label: \"TODO: the dead-end this negative repro proves must stay failing\"\n"
		files = append(files, File{Path: negativePath, Content: negativeRepro(p.ID)})
	}

	files = append([]File{{Path: recordPath, Content: recordSkeleton(p, kind, date, repros)}}, files...)
	return files, nil
}

func recordSkeleton(p Params, kind, date, repros string) string {
	return fmt.Sprintf(`---
schema_version: 1
id: %s
kind: %s
status: quarantined
title: %q
symptom:
  summary: "TODO: one-sentence observable symptom, in your own words"
applies_to:
  - ecosystem: "TODO: e.g. Go, Python, JavaScript, React Native"
    package: "TODO: the package, module, or runtime"
resolution:
  root_cause: "TODO: why it happens — re-derived from first principles + official docs"
  fix: "TODO: the escape, in your own words"
guard:
  repro: null
  repros:
%s  guarding_test: null
provenance:
  source: { author: %q, session: null, pr: null }
  recorded_at: %q
  valid: { from: %q, until: null }
  source_license: %q
  superseded_by: null
---

TODO: the authored narrative. Re-derive the fact from official docs + an executed
test; write symptom / resolution / dead-ends in your OWN words — never paste or
paraphrase a third-party snippet (docs/AUTHORING.md). source_url is intentionally
absent (authored, internal-only). Once the repro is real, run `+"`twiceshy doctor revalidate`"+`
and check the prose with `+"`twiceshy similarity`"+` before promotion.
`, p.ID, kind, p.Title, repros, p.Author, date, date, record.SourceLicenseAuthoredInternal)
}

func positiveRepro(id string) string {
	return fmt.Sprintf(`#!/usr/bin/env sh
# Positive (fail-to-pass) repro for %s — TODO: one line on what it proves.
#
# Fail-to-pass discipline (docs/SCHEMA.md, guard.repros):
#   exit 0  = trap reproduced AND escape works (record is valid)
#   exit 1  = the world changed (trap gone or escape broken) -> stale
#   exit 75 = environment cannot run the repro (skip, EX_TEMPFAIL)
#
# Write an ORIGINAL test (your own code) — the licensing firewall (ADR-0011 §5).
# Demonstrate the trap state, then the escape; never copy a third-party snippet.
set -u

# command -v <tool> >/dev/null 2>&1 || { echo "SKIP: <tool> unavailable"; exit 75; }

# TODO 1) reproduce the trap: the wrong/old way fails or misbehaves.
# TODO 2) show the escape: the right way works.

echo "TODO: implement the positive repro for %s"
exit 75
`, id, id)
}

func negativeRepro(id string) string {
	return fmt.Sprintf(`#!/usr/bin/env sh
# Negative repro for %s — TODO: the dead-end this proves must STAY failing.
#
# Negative discipline (docs/SCHEMA.md): a negative repro documents a dead-end
# ("don't try Z"); it must keep failing the way the record says.
#   exit 0  = the dead-end still fails as documented (record is valid)
#   exit 1  = the dead-end no longer fails (the record is stale)
#   exit 75 = environment cannot run the repro (skip, EX_TEMPFAIL)
set -u

# TODO: drive the dead-end and assert it still fails as the record documents.

echo "TODO: implement the negative repro for %s"
exit 75
`, id, id)
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
