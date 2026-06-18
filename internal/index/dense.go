// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/record"
)

// Dense retrieval (ADR-0009) is PULL-ONLY and CGO-free: pure-Go cosine over
// embeddings, fused with fingerprint + BM25 via RRF. It is never reachable from
// the hot/push path — Assess/Retrieve/Search take no Embedder, so they cannot
// embed. sqlite-vec is deliberately NOT used (it would break the CGO_ENABLED=0
// cross-compiled build); a brute-force scan is sub-millisecond at corpus scale.

// Embedder turns text into a dense vector. Pull-only: hold one to enable dense
// retrieval; pass nil (or let it error) and retrieval falls back to the
// embedding-free path with no error.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

const (
	// MatchedDense labels a hit that entered via dense (cosine) similarity.
	MatchedDense = "dense"
	// rrfK is the standard reciprocal-rank-fusion constant: fused score is
	// Σ 1/(rrfK + rank). Larger rrfK flattens the contribution of top ranks.
	rrfK = 60.0
	// denseFloor is a COARSE minimum cosine similarity for a dense candidate to
	// be eligible — the dense analogue of DefaultFloor: a heuristic to drop pure
	// noise, not a calibrated band (banding stays deferred, ADR-0006). Owned by a
	// guarding test.
	denseFloor = 0.5
	// denseCandidates bounds how many lexical/dense neighbours enter fusion.
	denseCandidates = 10
	// embedHTTPTimeout bounds a single embedding request.
	embedHTTPTimeout = 30 * time.Second
)

// OllamaEmbedder calls a local Ollama endpoint's /api/embeddings (stdlib HTTP,
// no new dependency, no CGO). It is the thin edge; the rest of dense is pure.
type OllamaEmbedder struct {
	Endpoint string
	Model    string
	Client   *http.Client
}

// NewOllamaEmbedder builds an embedder for endpoint (e.g. http://host:11434).
// An empty model defaults to nomic-embed-text (ADR-0001 §9).
func NewOllamaEmbedder(endpoint, model string) *OllamaEmbedder {
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaEmbedder{
		Endpoint: strings.TrimRight(endpoint, "/"),
		Model:    model,
		Client:   &http.Client{Timeout: embedHTTPTimeout},
	}
}

// Embed returns the embedding of text from Ollama's /api/embeddings.
func (o *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]string{"model": o.Model, "prompt": text})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.Endpoint+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ollama embed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ollama embed: decode: %w", err)
	}
	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("ollama embed: empty embedding")
	}
	return out.Embedding, nil
}

// embedText is the searchable projection of a record that gets embedded:
// title + symptom summary + error signatures + resolution. Body prose is left
// out to keep the vector focused on the symptom/fix, mirroring what search
// queries look like.
func embedText(r *record.Record) string {
	var b strings.Builder
	b.WriteString(r.Title)
	if r.Symptom != nil {
		if r.Symptom.Summary != "" {
			b.WriteString("\n")
			b.WriteString(r.Symptom.Summary)
		}
		for _, s := range r.Symptom.ErrorSignatures {
			b.WriteString("\n")
			b.WriteString(s)
		}
	}
	if r.Resolution != nil {
		if r.Resolution.RootCause != "" {
			b.WriteString("\n")
			b.WriteString(r.Resolution.RootCause)
		}
		if r.Resolution.Fix != "" {
			b.WriteString("\n")
			b.WriteString(r.Resolution.Fix)
		}
	}
	return strings.TrimSpace(b.String())
}

// EmbedCorpus computes and stores an embedding per record for dense retrieval.
// It is a no-op when emb is nil (dense disabled). Embeddings are cached by
// content hash in embedding_cache, so a Rebuild (e.g. on every server start)
// only re-embeds records whose text changed. Run AFTER Rebuild.
func (ix *Index) EmbedCorpus(ctx context.Context, recs []*record.Record, emb Embedder) error {
	if emb == nil {
		return nil
	}
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("embed corpus: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM embeddings"); err != nil {
		return fmt.Errorf("embed corpus: %w", err)
	}
	for _, r := range recs {
		text := embedText(r)
		if strings.TrimSpace(text) == "" {
			continue
		}
		h := hashText(text)
		var blob []byte
		err := tx.QueryRowContext(ctx, "SELECT vec FROM embedding_cache WHERE hash = ?", h).Scan(&blob)
		if err != nil { // cache miss (or error): embed and cache
			vec, eerr := emb.Embed(ctx, text)
			if eerr != nil {
				return fmt.Errorf("embed %s: %w", r.ID, eerr)
			}
			blob = encodeVec(vec)
			if _, err := tx.ExecContext(ctx,
				"INSERT OR REPLACE INTO embedding_cache (hash, vec) VALUES (?,?)", h, blob); err != nil {
				return fmt.Errorf("embed corpus: cache %s: %w", r.ID, err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT OR REPLACE INTO embeddings (record_id, vec) VALUES (?,?)", r.ID, blob); err != nil {
			return fmt.Errorf("embed corpus: store %s: %w", r.ID, err)
		}
	}
	return tx.Commit()
}

// RetrieveFused is the PULL-channel search: it applies the relevance floor like
// Retrieve, then fuses fingerprint-exact + BM25 + dense (cosine) hits via RRF.
// Fingerprint-exact precedence and the hard cap k≤3 are preserved. It falls
// back to the embedding-free Search — with no error — when emb is nil, the
// query can't be embedded, or no embeddings are stored. The hot/push path uses
// Assess/Retrieve (no Embedder) and never reaches this.
func (ix *Index) RetrieveFused(ctx context.Context, q Query, emb Embedder) ([]Hit, error) {
	q = floorPolicy(q)
	if emb == nil {
		return ix.Search(ctx, q)
	}
	qvec, err := emb.Embed(ctx, q.Text)
	if err != nil || len(qvec) == 0 {
		return ix.Search(ctx, q) // graceful fallback: dense is best-effort
	}

	k := q.K
	if k <= 0 || k > MaxK {
		k = MaxK
	}

	// Fingerprint-exact hits keep absolute precedence (a deterministic match
	// outranks any fused score), exactly as in Search.
	fp, err := ix.fingerprintHits(ctx, q, k)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, k)
	out := make([]Hit, 0, k)
	for _, h := range fp {
		if !seen[h.ID] {
			seen[h.ID] = true
			out = append(out, h)
			if len(out) >= k {
				return out, nil
			}
		}
	}

	// Fuse lexical + dense for the remaining slots.
	lex, err := ix.lexicalHits(ctx, q, denseCandidates)
	if err != nil {
		return nil, err
	}
	den, err := ix.denseHits(ctx, qvec, q, denseCandidates)
	if err != nil {
		return nil, err
	}
	for _, h := range rrfFuse(lex, den) {
		if !seen[h.ID] {
			seen[h.ID] = true
			out = append(out, h)
			if len(out) >= k {
				break
			}
		}
	}
	return out, nil
}

// denseHits scores every stored record embedding against qvec by cosine
// similarity, drops anything below the coarse denseFloor, and returns the top
// candidates (status/stack filtered like the other paths).
func (ix *Index) denseHits(ctx context.Context, qvec []float32, q Query, k int) ([]Hit, error) {
	var sb strings.Builder
	sb.WriteString(`SELECT e.vec, r.id, r.kind, r.status, r.title, r.summary, r.path
		FROM embeddings e JOIN records r ON r.id = e.record_id
		WHERE 1=1`)
	var args []any
	args = appendStatusFilter(&sb, args, q)
	args = appendStackFilter(&sb, args, q)

	rows, err := ix.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("dense search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var hits []Hit
	for rows.Next() {
		var blob []byte
		var h Hit
		if err := rows.Scan(&blob, &h.ID, &h.Kind, &h.Status, &h.Title, &h.Summary, &h.Path); err != nil {
			return nil, err
		}
		sim := cosine(qvec, decodeVec(blob))
		if sim < denseFloor {
			continue
		}
		h.Score = sim
		h.Matched = MatchedDense
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].ID < hits[j].ID
	})
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits, nil
}

// rrfFuse merges ranked hit lists by reciprocal-rank fusion: each hit's fused
// score is Σ 1/(rrfK + rank) over the lists it appears in (rank 1-based). The
// representative Hit is the first seen (lexical lists are passed before dense,
// so a hit found by both keeps its lexical Matched/Summary). Result is sorted
// by fused score, ties by id.
func rrfFuse(lists ...[]Hit) []Hit {
	score := map[string]float64{}
	rep := map[string]Hit{}
	var order []string
	for _, list := range lists {
		for rank, h := range list {
			if _, ok := rep[h.ID]; !ok {
				rep[h.ID] = h
				order = append(order, h.ID)
			}
			score[h.ID] += 1.0 / (rrfK + float64(rank+1))
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		if score[order[i]] != score[order[j]] {
			return score[order[i]] > score[order[j]]
		}
		return order[i] < order[j]
	})
	out := make([]Hit, 0, len(order))
	for _, id := range order {
		h := rep[id]
		h.Score = score[id]
		out = append(out, h)
	}
	return out
}

// cosine is the cosine similarity of two equal-length vectors; 0 for a length
// mismatch or a zero vector.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// encodeVec/decodeVec serialize a float32 vector as little-endian bytes for the
// embeddings BLOB column (pure-Go, no CGO).
func encodeVec(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func decodeVec(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
