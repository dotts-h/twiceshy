// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
)

// maxPRPages bounds each pagination loop: a server that ignores the page param
// (misconfig, behavior change) must fail loud instead of wedging a scheduled
// drain until the systemd kill — the judge-hang freeze class. At 50 items/page
// the bound allows 500k entries, far beyond any real backlog.
const maxPRPages = 10000

// paginated drives one empty-page-terminated pagination loop: fetch(page)
// reports how many items the page held. Termination is ONLY the empty page —
// a short-but-nonempty page is not the last one when the server's
// MAX_RESPONSE_ITEMS caps below the requested limit. Exhausting maxPRPages is
// an error, never a silent stop.
func paginated(what string, fetch func(page int) (int, error)) error {
	for page := 1; page <= maxPRPages; page++ {
		n, err := fetch(page)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
	return fmt.Errorf("%s pagination did not terminate within %d pages — refusing to spin (is the server ignoring the page param?)", what, maxPRPages)
}

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
			return sanitize(fmt.Errorf("GET %s returned status %d", urlStr, resp.StatusCode))
		}

		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return sanitize(err)
		}
		return nil
	}

	var prNumbers []int
	err := paginated("open-PR list", func(page int) (int, error) {
		urlStr := fmt.Sprintf("%s/pulls?state=open&limit=50&page=%d", apiRepo, page)
		var pulls []struct {
			Number int `json:"number"`
		}
		if err := getAndDecode(urlStr, &pulls); err != nil {
			return 0, err
		}
		for _, p := range pulls {
			prNumbers = append(prNumbers, p.Number)
		}
		return len(pulls), nil
	})
	if err != nil {
		return 0, err
	}

	maxID := 0
	for _, prNum := range prNumbers {
		err := paginated(fmt.Sprintf("PR #%d files", prNum), func(page int) (int, error) {
			urlStr := fmt.Sprintf("%s/pulls/%d/files?limit=50&page=%d", apiRepo, prNum, page)
			var files []struct {
				Filename string `json:"filename"`
			}
			if err := getAndDecode(urlStr, &files); err != nil {
				return 0, err
			}
			for _, file := range files {
				if n, ok := recordIDFromPath(file.Filename); ok && n > maxID {
					maxID = n
				}
			}
			return len(files), nil
		})
		if err != nil {
			return 0, err
		}
	}

	return maxID, nil
}

// ForgejoAPIFromOrigin resolves the repo API root and token for OpenPRMaxID.
// Env overrides are consulted FIRST — TWICESHY_FORGEJO_API (the
// scheme://host/api/v1 base), TWICESHY_FORGEJO_REPO (owner/repo), and
// TWICESHY_FORGEJO_TOKEN — and only the pieces still missing are derived from
// repoDir's remote.origin.url (the deployment convention: a token-embedded
// http(s) origin). Env-first is what makes the error guidance true: with API
// and REPO set the origin is never required, so a clone with an ssh or absent
// origin still works. A missing token alone is not fatal — the repo may be
// anonymously readable; a wrong guess fails loud downstream as a 401. getenv
// is threaded rather than read from the process env, per the cmd-layer DI
// convention.
func ForgejoAPIFromOrigin(ctx context.Context, repoDir string, getenv func(string) string) (apiRepo, token string, err error) {
	apiBase := strings.TrimSuffix(getenv("TWICESHY_FORGEJO_API"), "/")
	ownerRepo := strings.Trim(getenv("TWICESHY_FORGEJO_REPO"), "/")
	token = getenv("TWICESHY_FORGEJO_TOKEN")

	if apiBase == "" || ownerRepo == "" || token == "" {
		derived, derr := originParts(ctx, repoDir)
		if derr != nil && (apiBase == "" || ownerRepo == "") {
			return "", "", derr
		}
		if derr == nil {
			if apiBase == "" {
				apiBase = derived.apiBase
			}
			if ownerRepo == "" {
				ownerRepo = derived.ownerRepo
			}
			if token == "" {
				token = derived.token
			}
		}
	}
	return apiBase + "/repos/" + ownerRepo, token, nil
}

// originAPI is the derivable triple; token may legitimately be empty.
type originAPI struct {
	apiBase   string
	ownerRepo string
	token     string
}

// originParts derives the triple from repoDir's remote.origin.url. Errors name
// the env overrides — read before derivation, so they truly bypass this — and
// never echo the origin URL, which embeds the token (the #0149 bug class); the
// URL's path carries no credentials and is safe to name on the shape error.
func originParts(ctx context.Context, repoDir string) (originAPI, error) {
	const hatch = "set TWICESHY_FORGEJO_API + TWICESHY_FORGEJO_REPO (and TWICESHY_FORGEJO_TOKEN) to skip origin derivation"
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "config", "--get", "remote.origin.url")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return originAPI{}, fmt.Errorf("reading remote.origin.url: %v: %s (%s)", err, msg, hatch)
		}
		return originAPI{}, fmt.Errorf("reading remote.origin.url: %v (%s)", err, hatch)
	}

	u, err := url.Parse(strings.TrimSpace(string(out)))
	if err != nil {
		return originAPI{}, fmt.Errorf("origin URL does not parse as a URL (%s)", hatch)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return originAPI{}, fmt.Errorf("origin URL scheme %q is not http(s) (%s)", u.Scheme, hatch)
	}
	path := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return originAPI{}, fmt.Errorf("origin URL path %q is not owner/repo — a sub-path-mounted forge cannot be derived (%s)", path, hatch)
	}

	o := originAPI{
		apiBase:   u.Scheme + "://" + u.Host + "/api/v1",
		ownerRepo: parts[0] + "/" + parts[1],
	}
	if u.User != nil {
		if pw, ok := u.User.Password(); ok {
			o.token = pw
		} else {
			o.token = u.User.Username()
		}
	}
	return o, nil
}
