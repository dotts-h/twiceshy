// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// StatusCounts is a corpus-wide breakdown of record status counts, feeding the
// /statz "records" block (#0126). Total counts every record regardless of
// status; Validated/Quarantined are the two operationally interesting breakouts.
type StatusCounts struct {
	Validated   int
	Quarantined int
	Total       int
}

// RecordStatusCounts returns the corpus-wide record status breakdown.
func (ix *Index) RecordStatusCounts(ctx context.Context) (StatusCounts, error) {
	var sc StatusCounts
	err := ix.db.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(CASE WHEN status = 'validated' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN status = 'quarantined' THEN 1 ELSE 0 END), 0)
		 FROM records`).Scan(&sc.Total, &sc.Validated, &sc.Quarantined)
	if err != nil {
		return StatusCounts{}, fmt.Errorf("record status counts: %w", err)
	}
	return sc, nil
}

// TenantStat is one tenant token's usage summary for the operator /statz
// dashboard (#0126).
type TenantStat struct {
	ID         string
	Label      string
	Revoked    bool
	DailyQuota int
	CallsToday int
	Calls7d    int
	TopTools   map[string]int
}

// tenantStatsWindowDays is the trailing window (inclusive of today) TenantStats
// reports Calls7d and TopTools over.
const tenantStatsWindowDays = 7

// TenantStats returns every issued token's usage summary as of now: CallsToday
// is the same quota-tracked counter tenantAuth enforces (token_usage), while
// Calls7d/TopTools are a trailing 7-day (inclusive) per-tool breakdown from the
// finer-grained tenant_usage table (#0126).
func (ix *Index) TenantStats(ctx context.Context, now time.Time) ([]TenantStat, error) {
	day := now.UTC().Format("2006-01-02")
	rows, err := ix.db.QueryContext(ctx,
		`SELECT t.id, t.label, t.revoked_at, t.daily_quota, COALESCE(u.calls, 0)
		 FROM tokens t
		 LEFT JOIN token_usage u ON u.token_id = t.id AND u.day = ?
		 ORDER BY t.created_at`, day)
	if err != nil {
		return nil, fmt.Errorf("tenant stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []TenantStat
	for rows.Next() {
		var (
			s       TenantStat
			revoked sql.NullString
		)
		if err := rows.Scan(&s.ID, &s.Label, &revoked, &s.DailyQuota, &s.CallsToday); err != nil {
			return nil, fmt.Errorf("tenant stats: %w", err)
		}
		s.Revoked = revoked.Valid
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tenant stats: %w", err)
	}

	start := now.AddDate(0, 0, -(tenantStatsWindowDays - 1)).UTC().Format("2006-01-02")
	for i := range out {
		tools, calls7d, err := ix.tenantToolCounts(ctx, out[i].ID, start, day)
		if err != nil {
			return nil, err
		}
		out[i].TopTools = tools
		out[i].Calls7d = calls7d
	}
	return out, nil
}

// tenantToolCounts sums tenant_usage calls for id over [start, end] (inclusive
// UTC day strings, lexically comparable since both are YYYY-MM-DD), broken out
// per tool, plus the grand total across all tools in the window.
func (ix *Index) tenantToolCounts(ctx context.Context, id, start, end string) (map[string]int, int, error) {
	rows, err := ix.db.QueryContext(ctx,
		`SELECT tool, SUM(calls) FROM tenant_usage
		 WHERE token_id = ? AND day >= ? AND day <= ?
		 GROUP BY tool`, id, start, end)
	if err != nil {
		return nil, 0, fmt.Errorf("tenant tool counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tools := map[string]int{}
	total := 0
	for rows.Next() {
		var tool string
		var n int
		if err := rows.Scan(&tool, &n); err != nil {
			return nil, 0, fmt.Errorf("tenant tool counts: %w", err)
		}
		tools[tool] = n
		total += n
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("tenant tool counts: %w", err)
	}
	return tools, total, nil
}

// TopRecord is one record's usage ranking for the operator /statz dashboard
// (#0126): the busiest records by combined retrieved+pushed usage.
type TopRecord struct {
	ID        string
	Retrieved int
	Pushed    int
	Title     string
}

// TopRecords returns the n busiest records by retrieved+pushed usage,
// descending (ties broken by id, ascending). Records with no usage row are
// excluded entirely, not zero-padded in.
func (ix *Index) TopRecords(ctx context.Context, n int) ([]TopRecord, error) {
	if n <= 0 {
		return nil, nil
	}
	rows, err := ix.db.QueryContext(ctx,
		`SELECT u.record_id, u.retrieved, u.pushed, r.title
		 FROM usage u JOIN records r ON r.id = u.record_id
		 ORDER BY (u.retrieved + u.pushed) DESC, u.record_id ASC
		 LIMIT ?`, n)
	if err != nil {
		return nil, fmt.Errorf("top records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []TopRecord
	for rows.Next() {
		var tr TopRecord
		if err := rows.Scan(&tr.ID, &tr.Retrieved, &tr.Pushed, &tr.Title); err != nil {
			return nil, fmt.Errorf("top records: %w", err)
		}
		out = append(out, tr)
	}
	return out, rows.Err()
}
