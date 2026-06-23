// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"path/filepath"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/telemetry"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sessionIDFromRequest must survive the typed-nil gotcha: GetSession returns the
// *ServerSession boxed in a Session interface, so a nil pointer is a NON-nil interface.
// Dropping the concrete-pointer nil-check would panic on ss.ID(); a wrong type assertion
// would silently never extract.
func TestSessionIDFromRequest(t *testing.T) {
	if got := sessionIDFromRequest(nil); got != "" {
		t.Errorf("nil request → %q, want empty", got)
	}
	var nilSession *mcp.ServerSession
	if got := sessionIDFromRequest(&mcp.CallToolRequest{Session: nilSession}); got != "" {
		t.Errorf("typed-nil session → %q, want empty (must not panic)", got)
	}
	// A real (zero-value) *ServerSession is non-nil, so the id is actually read off it
	// (here "" — no transport session is wired) without panicking: the happy branch runs.
	if got := sessionIDFromRequest(&mcp.CallToolRequest{Session: &mcp.ServerSession{}}); got != "" {
		t.Errorf("zero-value session → %q, want empty", got)
	}
}

// recordSearchDecision attributes a search to its MCP session by a SALTED hash (never
// the raw id), so the retro helpfulness join can confirm only cards served in that
// session (#0069). A search with no session records no correlation key.
func TestRecordSearchDecision_AttributesSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.jsonl")
	tel, err := telemetry.NewRecorder(telemetry.Config{Path: path, Salt: []byte("salt")})
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{telemetry: tel}
	h.recordSearchDecision("q1", []index.Hit{{ID: "exp-0001", Score: 1}}, "session-xyz")
	h.recordSearchDecision("q2", []index.Hit{{ID: "exp-0002", Score: 1}}, "") // no session
	if err := tel.Close(); err != nil {
		t.Fatal(err)
	}

	served, err := telemetry.ServedInSession(path, tel.Hash("session-xyz"))
	if err != nil {
		t.Fatalf("ServedInSession: %v", err)
	}
	if len(served) != 1 || !served["exp-0001"] {
		t.Fatalf("session-xyz must be attributed exp-0001 only; got %v", served)
	}
	// The session-less search (exp-0002) is attributable to no session.
	if s, _ := telemetry.ServedInSession(path, tel.Hash("")); len(s) != 0 {
		t.Fatalf("a session-less search decision must not be attributable; got %v", s)
	}
}
