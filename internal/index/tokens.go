// SPDX-License-Identifier: AGPL-3.0-only

package index

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dotts-h/twiceshy/internal/entitlement"
)

// ErrTokenUnknown is returned when a token id or secret does not match.
var ErrTokenUnknown = errors.New("token unknown")

// ErrTokenRevoked is returned when a token has been revoked.
var ErrTokenRevoked = errors.New("token revoked")

// TokenInfo is metadata for an issued tenant token (never includes the secret).
type TokenInfo struct {
	ID             string
	Label          string
	CreatedAt      string
	RevokedAt      *string
	DailyQuota     int
	RatePerMin     int
	CallsToday     int
	OrganizationID string
	WorkspaceID    string
	Plan           entitlement.Plan
}

// IssueToken creates a new tenant token. The full bearer value is returned once.
func (ix *Index) IssueToken(label string, dailyQuota, ratePerMin int, now time.Time) (full string, id string, err error) {
	return ix.issueToken(label, dailyQuota, ratePerMin, now, nil)
}

type tokenAssignment struct {
	organizationID string
	workspaceID    string
	plan           entitlement.Plan
}

func (ix *Index) issueToken(label string, dailyQuota, ratePerMin int, now time.Time, assignment *tokenAssignment) (full string, id string, err error) {
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
	tx, err := ix.db.Begin()
	if err != nil {
		return "", "", fmt.Errorf("issue token: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`INSERT INTO tokens (id, secret_hash, label, created_at, daily_quota, rate_per_min)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, hash[:], label, created, dailyQuota, ratePerMin,
	); err != nil {
		return "", "", fmt.Errorf("issue token: %w", err)
	}
	if assignment != nil {
		if err := storeAssignment(tx, id, *assignment, created); err != nil {
			return "", "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", "", fmt.Errorf("issue token: commit: %w", err)
	}
	return full, id, nil
}

// IssuePlannedToken issues a token whose quotas are derived from plan
// entitlements and associates it with organization/workspace identity.
func (ix *Index) IssuePlannedToken(label, organizationID, workspaceID string, plan entitlement.Plan, now time.Time) (string, string, error) {
	if err := validateAssignment(organizationID, workspaceID); err != nil {
		return "", "", err
	}
	entitlements, err := entitlement.ForPlan(plan)
	if err != nil {
		return "", "", err
	}
	return ix.issueToken(label, entitlements.Quota.DailyCalls, entitlements.Quota.RatePerMinute, now,
		&tokenAssignment{organizationID: organizationID, workspaceID: workspaceID, plan: plan})
}

type sqlExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

func storeAssignment(exec sqlExecutor, tokenID string, assignment tokenAssignment, created string) error {
	if _, err := exec.Exec(`INSERT OR IGNORE INTO organizations (id, label, created_at) VALUES (?, ?, ?)`, assignment.organizationID, assignment.organizationID, created); err != nil {
		return fmt.Errorf("assign token plan: organization: %w", err)
	}
	if _, err := exec.Exec(`INSERT OR IGNORE INTO workspaces (id, organization_id, label, created_at) VALUES (?, ?, ?, ?)`, assignment.workspaceID, assignment.organizationID, assignment.workspaceID, created); err != nil {
		return fmt.Errorf("assign token plan: workspace: %w", err)
	}
	var owner string
	if err := exec.QueryRow(`SELECT organization_id FROM workspaces WHERE id = ?`, assignment.workspaceID).Scan(&owner); err != nil {
		return fmt.Errorf("assign token plan: workspace owner: %w", err)
	}
	if owner != assignment.organizationID {
		return fmt.Errorf("assign token plan: workspace %q belongs to another organization", assignment.workspaceID)
	}
	if _, err := exec.Exec(`INSERT INTO token_entitlements (token_id, organization_id, workspace_id, plan) VALUES (?, ?, ?, ?)
		ON CONFLICT(token_id) DO UPDATE SET organization_id=excluded.organization_id, workspace_id=excluded.workspace_id, plan=excluded.plan`, tokenID, assignment.organizationID, assignment.workspaceID, assignment.plan); err != nil {
		return fmt.Errorf("assign token plan: entitlement: %w", err)
	}
	return nil
}

func validateAssignment(organizationID, workspaceID string) error {
	for name, value := range map[string]string{"organization": organizationID, "workspace": workspaceID} {
		if value == "" || len(value) > 128 {
			return fmt.Errorf("%s id must be 1..128 characters", name)
		}
		for _, r := range value {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' && r != '.' {
				return fmt.Errorf("%s id %q contains an invalid character", name, value)
			}
		}
	}
	return nil
}

// AssignTokenPlan additively migrates an existing token onto a plan.
func (ix *Index) AssignTokenPlan(id, organizationID, workspaceID string, plan entitlement.Plan, now time.Time) error {
	if err := validateAssignment(organizationID, workspaceID); err != nil {
		return err
	}
	entitlements, err := entitlement.ForPlan(plan)
	if err != nil {
		return err
	}
	tx, err := ix.db.Begin()
	if err != nil {
		return fmt.Errorf("assign token plan: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`UPDATE tokens SET daily_quota = ?, rate_per_min = ? WHERE id = ?`, entitlements.Quota.DailyCalls, entitlements.Quota.RatePerMinute, id)
	if err != nil {
		return fmt.Errorf("assign token plan: token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTokenUnknown
	}
	if err := storeAssignment(tx, id, tokenAssignment{organizationID, workspaceID, plan}, now.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("assign token plan: commit: %w", err)
	}
	return nil
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
		        COALESCE(e.organization_id, ''), COALESCE(e.workspace_id, ''), COALESCE(e.plan, ''),
		        COALESCE(u.calls, 0)
		 FROM tokens t
		 LEFT JOIN token_entitlements e ON e.token_id = t.id
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
		if err := rows.Scan(&info.ID, &info.Label, &info.CreatedAt, &revoked, &info.DailyQuota, &info.RatePerMin, &info.OrganizationID, &info.WorkspaceID, &info.Plan, &calls); err != nil {
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
		`SELECT t.label, t.created_at, t.revoked_at, t.daily_quota, t.rate_per_min, t.secret_hash,
		        COALESCE(e.organization_id, ''), COALESCE(e.workspace_id, ''), COALESCE(e.plan, '')
		 FROM tokens t LEFT JOIN token_entitlements e ON e.token_id = t.id WHERE t.id = ?`, id,
	).Scan(&info.Label, &info.CreatedAt, &revoked, &info.DailyQuota, &info.RatePerMin, &hash, &info.OrganizationID, &info.WorkspaceID, &info.Plan)
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

// PlanReport returns token metadata for the gated operator report surface.
func (ix *Index) PlanReport(ctx context.Context) ([]TokenInfo, error) {
	rows, err := ix.db.QueryContext(ctx, `SELECT t.id, t.label, t.created_at, t.revoked_at, t.daily_quota, t.rate_per_min,
		COALESCE(e.organization_id, ''), COALESCE(e.workspace_id, ''), COALESCE(e.plan, '')
		FROM tokens t LEFT JOIN token_entitlements e ON e.token_id=t.id ORDER BY t.created_at`)
	if err != nil {
		return nil, fmt.Errorf("plan report: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TokenInfo
	for rows.Next() {
		var info TokenInfo
		var revoked sql.NullString
		if err := rows.Scan(&info.ID, &info.Label, &info.CreatedAt, &revoked, &info.DailyQuota, &info.RatePerMin, &info.OrganizationID, &info.WorkspaceID, &info.Plan); err != nil {
			return nil, fmt.Errorf("plan report: %w", err)
		}
		if revoked.Valid {
			value := revoked.String
			info.RevokedAt = &value
		}
		out = append(out, info)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("plan report: %w", err)
	}
	return out, nil
}

// CountTokenCall atomically increments today's call count for id, UNLESS it
// has already reached its tokens.daily_quota (<= 0 = unlimited), in which case
// the row is left unchanged. This used to increment unconditionally and let
// the caller compare after the fact (#0131 finding 2): under concurrent
// requests at the boundary, or simply many requests after quota was reached,
// that count-then-check let the stored total grow without bound even though
// every over-quota call was rejected. The cap check now lives in the same
// SQLite statement as the increment (the UPSERT's conditional WHERE), so
// there is no read-then-write race window in Go — verified under -race.
// Returns the resulting (possibly unchanged) total and whether this call was
// admitted.
func (ix *Index) CountTokenCall(id string, now time.Time) (calls int, allowed bool, err error) {
	day := now.UTC().Format("2006-01-02")
	err = ix.db.QueryRow(
		`INSERT INTO token_usage (token_id, day, calls) VALUES (?, ?, 1)
		 ON CONFLICT(token_id, day) DO UPDATE SET calls = calls + 1
		 WHERE (SELECT daily_quota FROM tokens WHERE id = ?) <= 0
		    OR calls < (SELECT daily_quota FROM tokens WHERE id = ?)
		 RETURNING calls`,
		id, day, id, id,
	).Scan(&calls)
	if errors.Is(err, sql.ErrNoRows) {
		// The conditional UPDATE was skipped: quota already reached today.
		// Read the (unchanged) stored total back for the caller to report.
		if selErr := ix.db.QueryRow(
			`SELECT calls FROM token_usage WHERE token_id = ? AND day = ?`,
			id, day,
		).Scan(&calls); selErr != nil {
			return 0, false, fmt.Errorf("count token call: quota check: %w", selErr)
		}
		return calls, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("count token call: %w", err)
	}
	return calls, true, nil
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

// CountContributionCall atomically increments tokenID's per-tool contribution
// counter for today (UTC), UNLESS it has already reached limit (<= 0 =
// unlimited), in which case the row is left unchanged. This is the
// enforcement-owned counterpart of CountTokenCall for the write-path
// contribution quota (ADR-0032, #0131 finding 2's pattern): the cap check and
// the increment live in the same conditional UPSERT, so there is no
// read-then-write race window in Go. It is distinct from, and does not read,
// the tenant_usage telemetry counter — that table stays pure best-effort
// observation and never gates a request again. Returns the resulting
// (possibly unchanged) total and whether this call was admitted.
func (ix *Index) CountContributionCall(tokenID, tool string, limit int, now time.Time) (calls int, allowed bool, err error) {
	day := now.UTC().Format("2006-01-02")
	err = ix.db.QueryRow(
		`INSERT INTO contribution_usage (token_id, day, tool, calls) VALUES (?, ?, ?, 1)
		 ON CONFLICT(token_id, day, tool) DO UPDATE SET calls = calls + 1
		 WHERE ? <= 0 OR calls < ?
		 RETURNING calls`,
		tokenID, day, tool, limit, limit,
	).Scan(&calls)
	if errors.Is(err, sql.ErrNoRows) {
		// The conditional UPDATE was skipped: limit already reached today.
		// Read the (unchanged) stored total back for the caller to report.
		if selErr := ix.db.QueryRow(
			`SELECT calls FROM contribution_usage WHERE token_id = ? AND day = ? AND tool = ?`,
			tokenID, day, tool,
		).Scan(&calls); selErr != nil {
			return 0, false, fmt.Errorf("count contribution call: quota check: %w", selErr)
		}
		return calls, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("count contribution call: %w", err)
	}
	return calls, true, nil
}

// TenantToolCallsToday returns tokenID's already-recorded tenant_usage count
// for tool today (UTC), WITHOUT incrementing — the read side of #0126's
// tool-keyed counter, telemetry read side only (ADR-0032 moved contribution
// quota enforcement to its own CountContributionCall/contribution_usage
// table; this method no longer gates any request).
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
