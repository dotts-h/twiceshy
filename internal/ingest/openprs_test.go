// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/ingest"
)

// OpenPRMaxID closes the #0121 collision window: merge-safe allocation against
// origin/main alone is blind to drafts sitting on open, unmerged PRs, so two
// parallel-open corpus PRs allocate the same id range (549 of 625 records in
// the 0105 drain). The scan asks the Forgejo API for every OPEN PR's changed
// files and returns the highest record id it finds, which the allocator treats
// as one more high-water mark.
func TestOpenPRMaxID(t *testing.T) {
	ctx := context.Background()

	t.Run("max across all open PRs, paginated past short pages", func(t *testing.T) {
		// One list item / file per page: a server whose page size is capped
		// below the requested limit returns SHORT-but-nonempty pages, so
		// only empty-page termination sees page 2 (the truncation trap).
		srv := newForgejoStub(t, "s3cret", map[int][]string{
			7: {
				"experience/2026/3197-colliding-a.md",
				"docs/notes.md",
				"experience/2026/3221-colliding-b.md",
			},
			9: {
				"experience/2027/0042-low.md",
				"experience/2026/README.md",
			},
		})
		defer srv.Close()

		got, err := ingest.OpenPRMaxID(ctx, srv.Client(), srv.URL+apiRepoPath, "s3cret")
		if err != nil {
			t.Fatalf("OpenPRMaxID: %v", err)
		}
		if got != 3221 {
			t.Errorf("got %d, want 3221 (max record id across every open PR's files, all pages)", got)
		}
	})

	t.Run("no open PRs yields zero", func(t *testing.T) {
		srv := newForgejoStub(t, "s3cret", nil)
		defer srv.Close()

		got, err := ingest.OpenPRMaxID(ctx, srv.Client(), srv.URL+apiRepoPath, "s3cret")
		if err != nil {
			t.Fatalf("OpenPRMaxID: %v", err)
		}
		if got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("API failure is an error, not a silent zero", func(t *testing.T) {
		// Degrading silently would recreate the invisible collision the
		// caller asked to be protected from — fail loud instead.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer srv.Close()

		if _, err := ingest.OpenPRMaxID(ctx, srv.Client(), srv.URL+apiRepoPath, "s3cret"); err == nil {
			t.Fatal("want an error when the Forgejo API fails, got nil")
		}
	})
}

// ForgejoAPIFromOrigin derives the repo API root and token from the corpus
// clone's token-embedded http(s) origin (the deployment convention), replacing
// the per-script sed parse that broke on userinfo URLs (#0149).
func TestForgejoAPIFromOrigin(t *testing.T) {
	ctx := context.Background()

	t.Run("derives api root and token from a token-embedded origin", func(t *testing.T) {
		repo := originRepo(t, "http://claude:s3cret@192.0.2.10:3030/claude/twiceshy-corpus.git")
		api, tok, err := ingest.ForgejoAPIFromOrigin(ctx, repo)
		if err != nil {
			t.Fatalf("ForgejoAPIFromOrigin: %v", err)
		}
		if want := "http://192.0.2.10:3030/api/v1/repos/claude/twiceshy-corpus"; api != want {
			t.Errorf("api = %q, want %q", api, want)
		}
		if tok != "s3cret" {
			t.Errorf("token = %q, want s3cret", tok)
		}
	})

	t.Run("env overrides win", func(t *testing.T) {
		repo := originRepo(t, "http://claude:s3cret@192.0.2.10:3030/claude/twiceshy-corpus.git")
		t.Setenv("TWICESHY_FORGEJO_API", "http://proxy.internal:9/api/v1")
		t.Setenv("TWICESHY_FORGEJO_TOKEN", "envtok")
		api, tok, err := ingest.ForgejoAPIFromOrigin(ctx, repo)
		if err != nil {
			t.Fatalf("ForgejoAPIFromOrigin: %v", err)
		}
		if want := "http://proxy.internal:9/api/v1/repos/claude/twiceshy-corpus"; api != want {
			t.Errorf("api = %q, want %q", api, want)
		}
		if tok != "envtok" {
			t.Errorf("token = %q, want envtok", tok)
		}
	})

	t.Run("non-http origin errors without echoing the token", func(t *testing.T) {
		repo := originRepo(t, "ssh://git@192.0.2.10/claude/twiceshy-corpus.git")
		if _, _, err := ingest.ForgejoAPIFromOrigin(ctx, repo); err == nil {
			t.Fatal("want an error for a non-http origin, got nil")
		}
	})

	t.Run("origin with no owner/repo path errors without echoing the token", func(t *testing.T) {
		repo := originRepo(t, "http://claude:s3cret@192.0.2.10:3030/")
		_, _, err := ingest.ForgejoAPIFromOrigin(ctx, repo)
		if err == nil {
			t.Fatal("want an error for an origin without owner/repo, got nil")
		}
		if strings.Contains(err.Error(), "s3cret") {
			t.Errorf("error %q leaks the origin-embedded token", err)
		}
	})
}

const apiRepoPath = "/api/v1/repos/claude/twiceshy-corpus"

// newForgejoStub serves the two Forgejo endpoints OpenPRMaxID uses — the open-PR
// list and each PR's changed files — one item per page regardless of the
// requested limit (emulating a server-side MAX_RESPONSE_ITEMS cap), and fails
// the test on a missing token header or a list request without state=open.
func newForgejoStub(t *testing.T, token string, prFiles map[int][]string) *httptest.Server {
	t.Helper()
	var prs []int
	for n := range prFiles {
		prs = append(prs, n)
	}
	// Deterministic order for paging.
	for i := 0; i < len(prs); i++ {
		for j := i + 1; j < len(prs); j++ {
			if prs[j] < prs[i] {
				prs[i], prs[j] = prs[j], prs[i]
			}
		}
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token "+token {
			t.Errorf("request %s lacks the token header: Authorization = %q", r.URL, got)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		page := 1
		if p := r.URL.Query().Get("page"); p != "" {
			n, err := strconv.Atoi(p)
			if err != nil {
				http.Error(w, "bad page", http.StatusBadRequest)
				return
			}
			page = n
		}
		switch {
		case r.URL.Path == apiRepoPath+"/pulls":
			if r.URL.Query().Get("state") != "open" {
				t.Errorf("pulls list must filter state=open, got query %q", r.URL.RawQuery)
			}
			type pr struct {
				Number int `json:"number"`
			}
			var out []pr
			if page <= len(prs) {
				out = append(out, pr{Number: prs[page-1]})
			}
			writeJSON(t, w, out)
		case strings.HasPrefix(r.URL.Path, apiRepoPath+"/pulls/") && strings.HasSuffix(r.URL.Path, "/files"):
			num := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, apiRepoPath+"/pulls/"), "/files")
			n, err := strconv.Atoi(num)
			if err != nil {
				http.Error(w, "bad pr number", http.StatusBadRequest)
				return
			}
			files := prFiles[n]
			type changed struct {
				Filename string `json:"filename"`
			}
			var out []changed
			if page <= len(files) {
				out = append(out, changed{Filename: files[page-1]})
			}
			writeJSON(t, w, out)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("encoding stub response: %v", err)
	}
}

// originRepo creates a bare-minimum git repo whose remote.origin.url is url.
func originRepo(t *testing.T, url string) string {
	t.Helper()
	repo := t.TempDir()
	gitNextID(t, repo, "init", "-q")
	gitNextID(t, repo, "remote", "add", "origin", url)
	return repo
}
