// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dotts-h/twiceshy/internal/record"
)

// maxPRPages bounds each pagination loop: a server that ignores the page param
// (misconfig, behavior change) must fail loud instead of wedging a scheduled
// drain until the systemd kill — the judge-hang freeze class. At 50 items/page
// the bound allows 500k entries, far beyond any real backlog.
const maxPRPages = 10000

// OpenPRMaxID lists all open pull requests for the repository, checks the changed
// files in each PR, and returns the maximum experience record ID found among them.
// It returns 0 if no experience files are found. Any API error or failure will return
// a sanitized error that does not expose the token.
func OpenPRMaxID(ctx context.Context, client *http.Client, apiRepo, token string) (int, error) {
	sanitize := func(err error) error {
		if err == nil || token == "" {
			return err
		}
		return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), token, "<token>"))
	}

	getAndDecode := func(urlStr string, target any) error {
		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return sanitize(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "token "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return sanitize(err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("GET %s returned status %d", urlStr, resp.StatusCode)
		}

		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return sanitize(err)
		}
		return nil
	}

	var prNumbers []int
	terminated := false
	for page := 1; page <= maxPRPages; page++ {
		urlStr := fmt.Sprintf("%s/pulls?state=open&limit=50&page=%d", apiRepo, page)
		var pulls []struct {
			Number int `json:"number"`
		}
		if err := getAndDecode(urlStr, &pulls); err != nil {
			return 0, err
		}
		if len(pulls) == 0 {
			terminated = true
			break
		}
		for _, p := range pulls {
			prNumbers = append(prNumbers, p.Number)
		}
	}
	if !terminated {
		return 0, fmt.Errorf("open-PR list pagination did not terminate within %d pages — refusing to spin (is the server ignoring the page param?)", maxPRPages)
	}

	maxID := 0
	for _, prNum := range prNumbers {
		terminated = false
		for page := 1; page <= maxPRPages; page++ {
			urlStr := fmt.Sprintf("%s/pulls/%d/files?limit=50&page=%d", apiRepo, prNum, page)
			var files []struct {
				Filename string `json:"filename"`
			}
			if err := getAndDecode(urlStr, &files); err != nil {
				return 0, err
			}
			if len(files) == 0 {
				terminated = true
				break
			}
			for _, file := range files {
				path := filepath.ToSlash(file.Filename)
				if !strings.HasPrefix(path, "experience/") {
					continue
				}
				id := filepath.Base(path)
				dash := strings.IndexByte(id, '-')
				if dash < 0 {
					continue
				}
				n, ok := record.Num("exp-" + id[:dash])
				if ok && n > maxID {
					maxID = n
				}
			}
		}
		if !terminated {
			return 0, fmt.Errorf("PR #%d files pagination did not terminate within %d pages — refusing to spin (is the server ignoring the page param?)", prNum, maxPRPages)
		}
	}

	return maxID, nil
}

// ForgejoAPIFromOrigin parses the repository's git remote origin URL to derive the Forgejo API base
// repository endpoint and the authentication token. If overrides are set via env variables, they take precedence.
// It returns a clean error if git fails or derivation is not possible, without echoing the origin URL.
func ForgejoAPIFromOrigin(ctx context.Context, repoDir string) (apiRepo, token string, err error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "config", "--get", "remote.origin.url")
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("git remote origin config check failed (set TWICESHY_FORGEJO_API and TWICESHY_FORGEJO_TOKEN to override)")
	}

	rawURL := strings.TrimSpace(string(out))
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("parsing origin URL failed (set TWICESHY_FORGEJO_API and TWICESHY_FORGEJO_TOKEN to override)")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", fmt.Errorf("unsupported origin URL scheme %q, only http/https are supported (set TWICESHY_FORGEJO_API and TWICESHY_FORGEJO_TOKEN to override)", u.Scheme)
	}

	path := strings.TrimPrefix(u.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("could not derive owner/repo from origin URL path (set TWICESHY_FORGEJO_API and TWICESHY_FORGEJO_TOKEN to override)")
	}
	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")

	var derivedToken string
	if u.User != nil {
		if password, hasPassword := u.User.Password(); hasPassword {
			derivedToken = password
		} else {
			derivedToken = u.User.Username()
		}
	}

	apiBase := fmt.Sprintf("%s://%s/api/v1", u.Scheme, u.Host)

	// Env overrides
	if envAPI := os.Getenv("TWICESHY_FORGEJO_API"); envAPI != "" {
		apiBase = envAPI
	}
	if envToken := os.Getenv("TWICESHY_FORGEJO_TOKEN"); envToken != "" {
		derivedToken = envToken
	}

	apiBase = strings.TrimSuffix(apiBase, "/")
	apiRepo = fmt.Sprintf("%s/repos/%s/%s", apiBase, owner, repo)
	return apiRepo, derivedToken, nil
}
