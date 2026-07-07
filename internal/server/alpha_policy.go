// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"fmt"
	"strings"

	"github.com/dotts-h/twiceshy/internal/record"
	"github.com/dotts-h/twiceshy/internal/screen"
)

// isAlphaTenant reports whether tenant is an untrusted alpha tok_ tenant
// (ADR-0030 phase 2) as opposed to "operator". Shared by every write-path
// hardening check added in #0128/ADR-0031 so they all agree on the same
// predicate.
func isAlphaTenant(tenant string) bool {
	return strings.HasPrefix(tenant, "tok_")
}

// alphaContributionQuotas is the single declaration point (ADR-0031, #0136)
// for every write tool's per-token, per-UTC-day contribution quota
// (#0128, ADR-0032) — separate from tenantAuth's per-call rate quota
// (#0125). Any NEW write surface MUST be added here AND to the
// alphaWriteSurfaces completeness list in alpha_policy_test.go — that test
// is the guard that makes "a write tool skipped the policy" a build failure,
// not a review catch. checkContributionQuota (tenant_usage.go) fails closed
// on a non-positive limit, so a missing entry can never be read as
// "unlimited" by omission.
var alphaContributionQuotas = map[string]int{
	"record_experience": 10,
	"report_outcome":    25,
	"report_issue":      25,
	"confirm_helpful":   50,
}

// Alpha size caps for report_outcome's/report_issue's free-text fields
// (#0128, ADR-0031) — tighter than the engine-wide guardrails, the same
// posture as record.go's alphaMax* caps below.
const (
	alphaMaxEvidenceBytes         = 8 << 10 // 8 KiB
	alphaMaxIssueDescriptionBytes = 8 << 10 // 8 KiB
)

// Tighter input-size caps for record_experience from an alpha tok_ tenant
// (#0128, ADR-0030 phase 2): untrusted contributors get a much smaller
// envelope than the operator's trusted-channel guardrails in record.go.
// title is already bounded to 8..120 runes by record.Validate, so it needs
// no separate cap here. Moved here (from record.go) by ADR-0031 so every
// alpha cap lives in one file.
const (
	alphaMaxBodyBytes      = 16 << 10 // 16 KiB
	alphaMaxSummaryBytes   = 2 << 10  // 2 KiB
	alphaMaxSignatures     = 10
	alphaMaxSignatureBytes = 500
)

// alphaStampAuthor returns the forced provenance origin for an alpha tenant
// — record.AlphaOriginPrefix + tenant, the push-eligibility gate's trust key
// — and the caller-supplied author trimmed for use as a display note. The
// caller-supplied author is NEVER the recorded origin: it can only ever
// appear as a note in the free-text body (#0128, ADR-0031).
func alphaStampAuthor(tenant, callerAuthor string) (recorded, display string) {
	return record.AlphaOriginPrefix + tenant, strings.TrimSpace(callerAuthor)
}

// secretFlags narrows findings to secret-category hazards, rendered as
// "secret:rule" tags — reuses screen's existing detectors/rule names, never a
// new pattern.
func secretFlags(findings []screen.Finding) []string {
	secrets := make([]screen.Finding, 0, len(findings))
	for _, f := range findings {
		if f.Category == "secret" {
			secrets = append(secrets, f)
		}
	}
	return screen.Flags(secrets)
}

// rejectAlphaSecrets scans texts for secret-shaped content and, if any is
// found, fails closed with an error naming tool and the matched rules
// (never the raw secret). For an alpha tenant, secret-shaped content is
// REJECTED outright, never redacted, never quarantined — the opposite
// posture of the operator channel, which still just quarantines with a
// security_flags entry (#0128, ADR-0031). Callers must run this BEFORE any
// id allocation, spooling, or record building so a rejected submission never
// lands anywhere.
func rejectAlphaSecrets(tool string, texts ...string) error {
	if findings := screen.Scan(texts...); screen.HasSecret(findings) {
		return fmt.Errorf("%s rejected: secret-shaped content detected (%s) — not stored",
			tool, strings.Join(secretFlags(findings), ", "))
	}
	return nil
}
