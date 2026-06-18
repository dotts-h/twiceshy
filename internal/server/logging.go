// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"errors"
	"strings"

	"github.com/dotts-h/twiceshy/internal/index"
)

// errorClass maps a handler error to a safe log label. Never log err.Error():
// FTS and validation messages can echo caller-supplied text.
func errorClass(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, index.ErrNotFound) {
		return "not_found"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "query must be non-empty"):
		return "invalid_query"
	case strings.Contains(msg, "query too large"),
		strings.Contains(msg, "body too large"),
		strings.Contains(msg, "title too large"),
		strings.Contains(msg, "too many error_signatures"),
		strings.Contains(msg, "error_signatures["):
		return "oversize"
	case strings.Contains(msg, "ingest: invalid draft"),
		strings.Contains(msg, "ingest: draft rejected"):
		return "invalid_arguments"
	default:
		return "internal"
	}
}

// clientError reports whether an error class represents a caller mistake
// rather than a server fault.
func clientError(class string) bool {
	switch class {
	case "not_found", "invalid_query", "oversize", "invalid_arguments":
		return true
	default:
		return false
	}
}
