package index

import "context"

// Novelty classifies an incoming symptom against the corpus.
type Novelty string

const (
	// NoveltyKnown means an exact fingerprint match was found: the symptom is deterministically present.
	NoveltyKnown Novelty = "known"
	// NoveltySimilar means a lexical near-match above the floor was found: a lead to verify, not proof.
	NoveltySimilar Novelty = "similar"
	// NoveltyNovel means nothing cleared the floor: the symptom is likely new.
	NoveltyNovel Novelty = "novel"
)

// Assessment is the verdict plus the evidence that justifies it.
type Assessment struct {
	Novelty    Novelty
	Candidates []Hit // the hits backing the verdict; empty for NoveltyNovel
}

// Assess runs the search pipeline once and classifies the result.
// It returns the novelty classification and the supporting evidence hits.
// Assess never touches the database directly, never re-ranks, and never mutates q.
func (ix *Index) Assess(ctx context.Context, q Query) (Assessment, error) {
	hits, err := ix.Search(ctx, q)
	if err != nil {
		return Assessment{}, err
	}

	// Check for fingerprint hits first.
	var fingerprints []Hit
	for _, h := range hits {
		if h.Matched == MatchedFingerprint {
			fingerprints = append(fingerprints, h)
		}
	}
	if len(fingerprints) > 0 {
		return Assessment{
			Novelty:    NoveltyKnown,
			Candidates: fingerprints,
		}, nil
	}

	// No fingerprints: check for lexical hits.
	if len(hits) > 0 {
		return Assessment{
			Novelty:    NoveltySimilar,
			Candidates: hits,
		}, nil
	}

	// No hits at all.
	return Assessment{
		Novelty:    NoveltyNovel,
		Candidates: nil,
	}, nil
}
