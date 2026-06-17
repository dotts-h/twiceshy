// SPDX-License-Identifier: AGPL-3.0-only

package server_test

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// L1: the search edge caps query *bytes*. index.maxQueryTokens bounds the token
// count, but a single whitespace-free multi-MB token slips past it into a
// SHA-256 plus a multi-MB FTS5 MATCH term — an authenticated client shouldn't
// turn one call into that much work.
func TestSearchRejectsOversizeQuery(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)
	huge := strings.Repeat("a", 17<<10) // one token, > maxQueryBytes (16 KiB)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_experience",
		Arguments: map[string]any{"query": huge},
	})
	if err == nil && !res.IsError {
		t.Error("an oversize query must be a tool error")
	}
}

// L2: record_experience caps its inputs before allocating an id and running the
// per-signature dedup probes.
func TestRecordRejectsOversizeInputs(t *testing.T) {
	ts := newTestServer(t)
	session := connect(t, ts)

	t.Run("too many signatures", func(t *testing.T) {
		sigs := make([]string, 64) // > maxRecordSignatures (32)
		for i := range sigs {
			sigs[i] = "sig"
		}
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "record_experience",
			Arguments: map[string]any{
				"kind": "trap", "title": "Trap with far too many signatures attached",
				"summary": "x", "error_signatures": sigs,
				"root_cause": "c", "fix": "f", "body": "b", "author": "claude",
			},
		})
		if err == nil && !res.IsError {
			t.Error("too many signatures must be a tool error")
		}
	})

	t.Run("oversize body", func(t *testing.T) {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name: "record_experience",
			Arguments: map[string]any{
				"kind": "trap", "title": "Trap with an enormous narrative body attached",
				"summary": "x", "error_signatures": []string{"unique-body-sig-1"},
				"root_cause": "c", "fix": "f",
				"body":   strings.Repeat("z", (64<<10)+1), // > maxRecordBodyBytes
				"author": "claude",
			},
		})
		if err == nil && !res.IsError {
			t.Error("oversize body must be a tool error")
		}
	})
}
