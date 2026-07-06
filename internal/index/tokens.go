// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrTokenUnknown is returned when a token id or secret does not match.
var ErrTokenUnknown = errors.New("token unknown")

// ErrTokenRevoked is returned when a token has been revoked.
var ErrTokenRevoked = errors.New("token revoked")

// TokenInfo is metadata for an issued tenant token (never includes the secret).
type TokenInfo struct {
	ID         string
	Label      string
	CreatedAt  string
	RevokedAt  *string
	DailyQuota int
	RatePerMin int
	CallsToday int
}

// IssueToken creates a new tenant token. The full bearer value is returned once.
func (ix *Index) IssueToken(label string, dailyQuota, ratePerMin int, now time.Time) (full string, id string, err error) {
	var idBytes [4]byte
	var secretBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return "", "", fmt.Errorf("issue token: id entropy: %w", err)
	}
	if _, err := rand.Read(secretBytes[:]); err != nil {
		return "", "", fmt.Errorf("issue token: secret entropy: %w", err)
	}
	id = "tok_" + hex.EncodeToString(idBytes[:])
	secret := hex.EncodeToString(secretBytes[:])
	full = id + "_" + secret
	hash := sha256.Sum256([]byte(secret))
	created := now.UTC().Format(time.RFC3339)
	if _, err := ix.db.Exec(
		`INSERT INTO tokens (id, secret_hash, label, created_at, daily_quota, rate_per_min)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, hash[:], label, created, dailyQuota, ratePerMin,
	); err != nil {
		return "", "", fmt.Errorf("issue token: %w", err)
	}
	return full, id, nil
}

// RevokeToken marks a token revoked at now.
func (ix *Index) RevokeToken(id string, now time.Time) error {
	revoked := now.UTC().Format(time.RFC3339)
	res, err := ix.db.Exec(`UPDATE tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, revoked, id)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	if n == 0 {
		var exists int
		err := ix.db.QueryRow(`SELECT 1 FROM tokens WHERE id = ?`, id).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTokenUnknown
		}
		if err != nil {
			return fmt.Errorf("revoke token: %w", err)
		}
		return ErrTokenRevoked
	}
	return nil
}

// ListTokens returns all tokens with today's call count (UTC).
func (ix *Index) ListTokens(now time.Time) ([]TokenInfo, error) {
	day := now.UTC().Format("2006-01-02")
	rows, err := ix.db.Query(
		`SELECT t.id, t.label, t.created_at, t.revoked_at, t.daily_quota, t.rate_per_min,
		        COALESCE(u.calls, 0)
		 FROM tokens t
		 LEFT JOIN token_usage u ON u.token_id = t.id AND u.day = ?
		 ORDER BY t.created_at`,
		day,
	)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []TokenInfo
	for rows.Next() {
		var (
			info    TokenInfo
			revoked sql.NullString
			calls   int
		)
		if err := rows.Scan(&info.ID, &info.Label, &info.CreatedAt, &revoked, &info.DailyQuota, &info.RatePerMin, &calls); err != nil {
			return nil, fmt.Errorf("list tokens: %w", err)
		}
		if revoked.Valid {
			s := revoked.String
			info.RevokedAt = &s
		}
		info.CallsToday = calls
		out = append(out, info)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	return out, nil
}

// AuthenticateToken validates a full bearer token value.
func (ix *Index) AuthenticateToken(full string, now time.Time) (TokenInfo, error) {
	id, secret, ok := parseTokenFull(full)
	if !ok {
		return TokenInfo{}, ErrTokenUnknown
	}
	var (
		info    TokenInfo
		hash    []byte
		revoked sql.NullString
	)
	err := ix.db.QueryRow(
		`SELECT label, created_at, revoked_at, daily_quota, rate_per_min, secret_hash
		 FROM tokens WHERE id = ?`, id,
	).Scan(&info.Label, &info.CreatedAt, &revoked, &info.DailyQuota, &info.RatePerMin, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return TokenInfo{}, ErrTokenUnknown
	}
	if err != nil {
		return TokenInfo{}, fmt.Errorf("authenticate token: %w", err)
	}
	info.ID = id
	if revoked.Valid {
		s := revoked.String
		info.RevokedAt = &s
		return TokenInfo{}, ErrTokenRevoked
	}
	want := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(hash, want[:]) != 1 {
		return TokenInfo{}, ErrTokenUnknown
	}
	day := now.UTC().Format("2006-01-02")
	if err := ix.db.QueryRow(
		`SELECT COALESCE((SELECT calls FROM token_usage WHERE token_id = ? AND day = ?), 0)`,
		id, day,
	).Scan(&info.CallsToday); err != nil {
		return TokenInfo{}, fmt.Errorf("authenticate token: usage: %w", err)
	}
	return info, nil
}

// CountTokenCall atomically increments today's call count and returns the new total.
func (ix *Index) CountTokenCall(id string, now time.Time) (int, error) {
	day := now.UTC().Format("2006-01-02")
	var calls int
	err := ix.db.QueryRow(
		`INSERT INTO token_usage (token_id, day, calls) VALUES (?, ?, 1)
		 ON CONFLICT(token_id, day) DO UPDATE SET calls = calls + 1
		 RETURNING calls`,
		id, day,
	).Scan(&calls)
	if err != nil {
		return 0, fmt.Errorf("count token call: %w", err)
	}
	return calls, nil
}

// CountTenantCall atomically increments today's per-tenant, per-tool call
// counter (#0126): tokenID is "operator" or a tok_ id, tool is the MCP tool
// name or "push"/"retro". This is a telemetry counter, distinct from
// CountTokenCall's quota enforcement — callers must log and continue on error,
// never fail the request over it.
func (ix *Index) CountTenantCall(tokenID, tool string, now time.Time) error {
	day := now.UTC().Format("2006-01-02")
	if _, err := ix.db.Exec(
		`INSERT INTO tenant_usage (token_id, day, tool, calls) VALUES (?, ?, ?, 1)
		 ON CONFLICT(token_id, day, tool) DO UPDATE SET calls = calls + 1`,
		tokenID, day, tool,
	); err != nil {
		return fmt.Errorf("count tenant call: %w", err)
	}
	return nil
}

// TenantToolCallsToday returns tokenID's already-recorded tenant_usage count
// for tool today (UTC), WITHOUT incrementing — the read side of #0126's
// tool-keyed counter, reused by the alpha-tenant contribution-quota gate
// (#0128). withTenantTelemetry bumps this same counter via CountTenantCall
// before every tool handler runs, so by the time a handler reads it here, the
// current call is already included in the count.
func (ix *Index) TenantToolCallsToday(tokenID, tool string, now time.Time) (int, error) {
	day := now.UTC().Format("2006-01-02")
	var calls int
	if err := ix.db.QueryRow(
		`SELECT COALESCE((SELECT calls FROM tenant_usage WHERE token_id = ? AND day = ? AND tool = ?), 0)`,
		tokenID, day, tool,
	).Scan(&calls); err != nil {
		return 0, fmt.Errorf("tenant tool calls today: %w", err)
	}
	return calls, nil
}

func parseTokenFull(full string) (id, secret string, ok bool) {
	if !strings.HasPrefix(full, "tok_") {
		return "", "", false
	}
	rest := full[4:]
	underscore := strings.IndexByte(rest, '_')
	if underscore != 8 || len(rest) <= 9 {
		return "", "", false
	}
	id = "tok_" + rest[:8]
	secret = rest[9:]
	if len(secret) != 32 {
		return "", "", false
	}
	for _, c := range secret {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", "", false
		}
	}
	for _, c := range rest[:8] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", "", false
		}
	}
	return id, secret, true
}
