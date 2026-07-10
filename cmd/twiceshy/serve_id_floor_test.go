// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// #0150: container deployments may not have a derivable HTTP origin. The live
// server's floor resolver must honor the Forgejo env triple before consulting
// git, just like the batch allocation paths.
func TestServeIDFloorResolverUsesEnvFirstForgejoConfig(t *testing.T) {
	const token = "floor-test-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token "+token {
			t.Fatalf("Authorization = %q", got)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		switch {
		case r.URL.Path == "/api/v1/repos/owner/corpus/pulls" && page == 1:
			_ = json.NewEncoder(w).Encode([]map[string]any{{"number": 7}})
		case r.URL.Path == "/api/v1/repos/owner/corpus/pulls/7/files" && page == 1:
			_ = json.NewEncoder(w).Encode([]map[string]any{{"filename": "experience/2026/4550-open.md"}})
		default:
			_ = json.NewEncoder(w).Encode([]any{})
		}
	}))
	t.Cleanup(srv.Close)

	env := map[string]string{
		"TWICESHY_FORGEJO_API":   srv.URL + "/api/v1",
		"TWICESHY_FORGEJO_REPO":  "owner/corpus",
		"TWICESHY_FORGEJO_TOKEN": token,
	}
	resolve := serveIDFloorResolver(t.TempDir(), func(key string) string { return env[key] })
	got, err := resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != 4550 {
		t.Fatalf("floor = %d, want 4550", got)
	}
}
