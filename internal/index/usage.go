// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dotts-h/twiceshy/internal/record"
)

// Usage counters are the closed loop's reinforcement signal, distinct from
// execution proof (ADR-0013 §4): retrieval bumps `retrieved`/`last_hit`, a
// positive outcome report bumps `confirmed_helpful`. They live in a dedicated
// `usage` table — the one piece of index state that is NOT derived from the
// markdown corpus — so a Rebuild (which wipes and reloads records/fts) leaves
// them untouched (the table is absent from Rebuild's DELETE set), exactly like
// the embedding cache. Materializing them back into `provenance.usage` for
// full-reset durability and the D4 lifecycle doctor is deferred (ADR-0010).
//
// Every counter is monotonic: writes are `+1` deltas via ON CONFLICT, so
// concurrent retrievals never lose an update (guarded by a -race test).

// RecordHits increments `retrieved` and sets `last_hit` (a "YYYY-MM-DD" date)
// for each served record id, in one transaction. It is off the hot path: the
// server calls it asynchronously so a retrieval's latency budget is unaffected.
// An empty id list is a no-op.
func (ix *Index) RecordHits(ctx context.Context, ids []string, date string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("usage: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO usage (record_id, retrieved, last_hit) VALUES (?, 1, ?)
			 ON CONFLICT(record_id) DO UPDATE SET
			   retrieved = retrieved + 1,
			   last_hit  = excluded.last_hit`,
			id, date); err != nil {
			return fmt.Errorf("usage: record hit %s: %w", id, err)
		}
	}
	return tx.Commit()
}

// RecordPushes increments `pushed` for each record auto-injected via the push
// channel (POST /push), in one transaction. Like RecordHits it is off the hot
// path (the server calls it asynchronously) and a no-op on an empty list. It
// deliberately does NOT touch `retrieved` or `last_hit`: a push impression is a
// distinct, weaker signal than a deliberate pull, and `last_hit` stays tied to
// genuine retrieval so staleness is not muddied by unprompted injections. The
// closed loop is `pushed` (denominator) vs `confirmed_helpful` (numerator).
func (ix *Index) RecordPushes(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := ix.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("usage: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO usage (record_id, pushed) VALUES (?, 1)
			 ON CONFLICT(record_id) DO UPDATE SET pushed = pushed + 1`,
			id); err != nil {
			return fmt.Errorf("usage: record push %s: %w", id, err)
		}
	}
	return tx.Commit()
}

// ConfirmHelpful increments `confirmed_helpful` for a record — the positive
// signal a `report_outcome` (#0031) sets when a served lesson actually worked.
// It does not touch `retrieved`/`last_hit`.
func (ix *Index) ConfirmHelpful(ctx context.Context, id string) error {
	if _, err := ix.db.ExecContext(ctx,
		`INSERT INTO usage (record_id, confirmed_helpful) VALUES (?, 1)
		 ON CONFLICT(record_id) DO UPDATE SET confirmed_helpful = confirmed_helpful + 1`,
		id); err != nil {
		return fmt.Errorf("usage: confirm helpful %s: %w", id, err)
	}
	return nil
}

// Usage returns the accumulated usage for a record. A record never retrieved
// (no row) has the zero value — not an error — so callers see a clean
// {retrieved: 0, confirmed_helpful: 0, last_hit: nil}.
func (ix *Index) Usage(ctx context.Context, id string) (record.Usage, error) {
	var (
		u       record.Usage
		lastHit sql.NullString
	)
	err := ix.db.QueryRowContext(ctx,
		`SELECT retrieved, pushed, confirmed_helpful, last_hit FROM usage WHERE record_id = ?`, id).
		Scan(&u.Retrieved, &u.Pushed, &u.ConfirmedHelpful, &lastHit)
	if errors.Is(err, sql.ErrNoRows) {
		return record.Usage{}, nil
	}
	if err != nil {
		return record.Usage{}, fmt.Errorf("usage: read %s: %w", id, err)
	}
	if lastHit.Valid {
		u.LastHit = &lastHit.String
	}
	return u, nil
}

// AllUsage returns every record's accumulated usage, keyed by record id. Records
// with no usage row are simply absent from the map.
func (ix *Index) AllUsage(ctx context.Context) (map[string]record.Usage, error) {
	rows, err := ix.db.QueryContext(ctx,
		`SELECT record_id, retrieved, pushed, confirmed_helpful, last_hit FROM usage`)
	if err != nil {
		return nil, fmt.Errorf("usage: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]record.Usage)
	for rows.Next() {
		var (
			id      string
			u       record.Usage
			lastHit sql.NullString
		)
		if err := rows.Scan(&id, &u.Retrieved, &u.Pushed, &u.ConfirmedHelpful, &lastHit); err != nil {
			return nil, fmt.Errorf("usage: scan: %w", err)
		}
		if lastHit.Valid {
			u.LastHit = &lastHit.String
		}
		out[id] = u
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usage: rows: %w", err)
	}
	return out, nil
}
