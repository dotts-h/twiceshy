// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/record"
)

// Draft is an agent-proposed record before it is identified, dated, or sited.
type Draft struct {
	Kind       string
	Title      string
	Symptom    *record.Symptom
	AppliesTo  []record.AppliesTo
	Resolution *record.Resolution
	Guard      *record.Guard
	Body       string
}

// Meta is the caller-supplied identity/provenance the draft itself can't carry.
type Meta struct {
	ID      string // pre-allocated record id, e.g. "exp-0042"
	Author  string
	Session *string
	Now     string // "YYYY-MM-DD"
}

// Outcome is the ingest decision plus its evidence.
type Outcome struct {
	Novelty    index.Novelty
	Candidates []index.Hit
	Record     *record.Record // the quarantined record to persist; nil when Known
}

// Prepare ingests a draft experience record. It deduplicates against the corpus
// using the index, and if the draft is not an exact known duplicate, it builds a
// fully-provenanced, schema-valid record forced into "quarantined" status.
//
// Dedup probes every distinct signal the draft carries — each error signature,
// then the summary (the title only when there is no symptom at all). The index
// fingerprints every signature, so probing only the first would miss exact
// duplicates keyed on a later one. Any Known hit on any probe is terminal: the
// draft is a duplicate and no record is created. Otherwise the strongest verdict
// (Similar over Novel) and its candidates are carried through.
//
// On a Known outcome the returned Record is nil; otherwise a quarantined record
// is returned after schema validation.
func Prepare(ctx context.Context, ix *index.Index, repo string, d Draft, m Meta) (Outcome, error) {
	novelty := index.NoveltyNovel
	var candidates []index.Hit
	for _, probe := range probes(d) {
		a, err := ix.Assess(ctx, index.Query{Text: probe, Repo: repo})
		if err != nil {
			return Outcome{}, err
		}
		if a.Novelty == index.NoveltyKnown {
			return Outcome{Novelty: index.NoveltyKnown, Candidates: a.Candidates, Record: nil}, nil
		}
		if a.Novelty == index.NoveltySimilar && novelty == index.NoveltyNovel {
			novelty = index.NoveltySimilar
			candidates = a.Candidates
		}
	}

	rec := &record.Record{
		SchemaVersion: record.SchemaVersion,
		ID:            m.ID,
		Kind:          d.Kind,
		Status:        "quarantined",
		Title:         d.Title,
		Symptom:       d.Symptom,
		AppliesTo:     d.AppliesTo,
		Resolution:    d.Resolution,
		Guard:         d.Guard,
		Body:          strings.TrimSpace(d.Body),
		Provenance: record.Provenance{
			Source: record.Source{
				Author:  m.Author,
				Session: m.Session,
				PR:      nil,
			},
			RecordedAt:  m.Now,
			ValidatedAt: nil,
			Valid: record.Validity{
				From:  m.Now,
				Until: nil,
			},
			SupersededBy: nil,
			Usage:        nil,
		},
		Path: buildPath(m.Now, m.ID, d.Title),
	}

	if err := record.Validate(rec); err != nil {
		return Outcome{}, fmt.Errorf("ingest: invalid draft: %w", err)
	}

	return Outcome{
		Novelty:    novelty,
		Candidates: candidates,
		Record:     rec,
	}, nil
}

// probes returns the dedup probes for a draft, strongest signal first: each
// non-empty error signature, then the summary. With no symptom at all (e.g. a
// convention or workflow record), the title is the only available signal.
func probes(d Draft) []string {
	var ps []string
	if d.Symptom != nil {
		for _, sig := range d.Symptom.ErrorSignatures {
			if strings.TrimSpace(sig) != "" {
				ps = append(ps, sig)
			}
		}
		if s := strings.TrimSpace(d.Symptom.Summary); s != "" {
			ps = append(ps, s)
		}
	}
	if len(ps) == 0 {
		if t := strings.TrimSpace(d.Title); t != "" {
			ps = append(ps, t)
		}
	}
	return ps
}

// buildPath constructs the file path for a record:
// experience/<year>/<num>-<slug>.md. It is total — a malformed now/id/title
// yields an invalid path that record.Validate rejects, never a panic.
func buildPath(now, id, title string) string {
	year := now
	if len(now) >= 4 {
		year = now[:4]
	}
	num := strings.TrimPrefix(id, "exp-")
	slug := slugify(title)
	if slug == "" {
		slug = "record" // keep the path valid for titles with no [a-z0-9] runes
	}
	return fmt.Sprintf("experience/%s/%s-%s.md", year, num, slug)
}

// slugify converts a title into a URL-safe slug.
// Lowercases, collapses runs of non-[a-z0-9] to a single "-", trims leading/trailing "-".
func slugify(title string) string {
	title = strings.ToLower(title)
	var b strings.Builder
	prevDash := false
	for _, r := range title {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			prevDash = false
		} else if unicode.IsPrint(r) {
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	slug := b.String()
	slug = strings.Trim(slug, "-")
	return slug
}
