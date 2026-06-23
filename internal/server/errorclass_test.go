// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"errors"
	"fmt"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
)

// errorClass is the seam that prevents echoing caller-supplied FTS/validation
// text into logs (logging.go: "Never log err.Error()"). The substring matches
// are brittle by nature, so pin every branch directly. The error strings below
// mirror the actual product messages (server.go, record.go, ingest), so a
// refactor that changes a message without updating errorClass regresses here.
func TestErrorClass(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"not_found wrapped", fmt.Errorf("get exp-9999: %w", index.ErrNotFound), "not_found"},

		{"invalid_query", errors.New("query must be non-empty"), "invalid_query"},

		{"oversize query", fmt.Errorf("query too large: %d bytes (max %d)", 20000, 16384), "oversize"},
		{"oversize body", fmt.Errorf("body too large: %d bytes (max %d)", 70000, 65536), "oversize"},
		{"oversize title", fmt.Errorf("title too large: %d bytes (max %d)", 5000, 256), "oversize"},
		{"oversize too many signatures", fmt.Errorf("too many error_signatures: %d (max %d)", 64, 32), "oversize"},
		{"oversize signature element", fmt.Errorf("error_signatures[%d] too large: %d bytes (max %d)", 0, 9000, 4096), "oversize"},

		{"invalid_arguments draft", errors.New("ingest: invalid draft: trap without resolution"), "invalid_arguments"},
		{"invalid_arguments rejected", errors.New("ingest: draft rejected by safety gate: prompt-injection markers"), "invalid_arguments"},

		{"internal fallthrough", errors.New("disk on fire"), "internal"},
		// A caller-supplied FTS error must NOT be classified as a known caller
		// class — it falls through to internal, and its text is never the label.
		{"internal unknown caller text", errors.New(`FTS5: syntax error near "."`), "internal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorClass(tc.err); got != tc.want {
				t.Errorf("errorClass(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// clientError decides WARN (caller mistake) vs ERROR (server fault). The class
// strings are exactly those errorClass can return.
func TestClientError(t *testing.T) {
	for _, tc := range []struct {
		class string
		want  bool
	}{
		{"not_found", true},
		{"invalid_query", true},
		{"oversize", true},
		{"invalid_arguments", true},
		{"internal", false},
		{"", false},
		{"unrecognized", false},
	} {
		t.Run(tc.class, func(t *testing.T) {
			if got := clientError(tc.class); got != tc.want {
				t.Errorf("clientError(%q) = %v, want %v", tc.class, got, tc.want)
			}
		})
	}
}

// TestBearerRejectReason pins the security-sensitive auth predicate directly,
// without depending on the MCP handler's response code. "" means accept; any
// non-empty string is the audit reason logged on a 401. The constant-time
// content branch (bad_token) is only the deciding factor when the candidate
// token is the SAME length as the real one — exercised explicitly here.
func TestBearerRejectReason(t *testing.T) {
	const (
		realToken = "s3cret-test-token" // 17 bytes
		prefix    = "Bearer "
	)
	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"missing", "", "missing_bearer"},
		{"wrong scheme basic", "Basic " + realToken, "wrong_scheme"},
		{"empty bearer (prefix only)", "Bearer ", "wrong_scheme"},
		{"different-length wrong token", "Bearer wrong", "bad_token"},
		// Same byte-length as realToken, differing only in the last byte: this is
		// the case the constant-time compare specifically guards.
		{"same-length wrong token", "Bearer s3cret-test-tokeX", "bad_token"},
		{"correct token", "Bearer " + realToken, ""},
		// Scheme is matched case-insensitively (strings.EqualFold).
		{"case-folded scheme correct token", "bearer " + realToken, ""},
		{"upper-case scheme correct token", "BEARER " + realToken, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := bearerRejectReason(tc.got, realToken, prefix); got != tc.want {
				t.Errorf("bearerRejectReason(%q) = %q, want %q", tc.got, got, tc.want)
			}
		})
	}
}
