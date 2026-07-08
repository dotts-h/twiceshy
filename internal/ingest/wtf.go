// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/dotts-h/twiceshy/internal/record"
)

const (
	wtfjsRawURL         = "https://raw.githubusercontent.com/denysdovhan/wtfjs/master/README.md"
	wtfpythonRawURL     = "https://raw.githubusercontent.com/satwikkansal/wtfpython/master/README.md"
	wtfjsSourceBase     = "https://github.com/denysdovhan/wtfjs/blob/master/README.md"
	wtfpythonSourceBase = "https://github.com/satwikkansal/wtfpython/blob/master/README.md"
	wtfSourceLicense    = "WTFPL"
)

// MaxWtfDrafts defensively caps drafts from both WTF collections in one Drafts()
// call; `twiceshy ingest -limit` at import time is the primary gate.
const MaxWtfDrafts = 200

var defaultWtfTargets = []string{"wtfjs", "wtfpython"}

var (
	wtfFenceRe         = regexp.MustCompile("(?is)```(?:js|javascript|py)\\s*\n(.*?)```")
	wtfjsExplainRe     = regexp.MustCompile(`(?is)###\s*💡\s*Explanation:?\s*\n(.*)`)
	wtfpythonExplainRe = regexp.MustCompile(`(?is)####\s*💡\s*Explanation:?\s*\n(.*)`)
	wtfArrowResultRe   = regexp.MustCompile(`//\s*->\s*(.+)`)
)

// WtfSource imports curated JS/Python gotcha entries from the WTFPL-licensed
// wtfjs and wtfpython README collections (#0134). Every draft is born
// quarantined; prose is license-clean under WTFPL.
type WtfSource struct {
	targets []string
	fetch   func(ctx context.Context, target string) (io.ReadCloser, error)
}

// WtfOption configures a WtfSource.
type WtfOption func(*WtfSource)

// WithWtfFetch overrides the per-target fetcher (tests inject fixtures; production
// hits raw.githubusercontent.com).
func WithWtfFetch(fetch func(ctx context.Context, target string) (io.ReadCloser, error)) WtfOption {
	return func(s *WtfSource) { s.fetch = fetch }
}

// WithWtfTargets sets which collections to fetch ("wtfjs", "wtfpython"); empty is ignored.
func WithWtfTargets(targets []string) WtfOption {
	return func(s *WtfSource) {
		if len(targets) > 0 {
			s.targets = targets
		}
	}
}

// NewWtfSource returns a live wtfjs/wtfpython gotcha importer over the default target set.
func NewWtfSource(opts ...WtfOption) Source {
	s := &WtfSource{targets: defaultWtfTargets}
	for _, opt := range opts {
		opt(s)
	}
	if s.fetch == nil {
		s.fetch = wtfFetcher()
	}
	return s
}

func wtfFetcher() func(context.Context, string) (io.ReadCloser, error) {
	urls := map[string]string{
		"wtfjs":     wtfjsRawURL,
		"wtfpython": wtfpythonRawURL,
	}
	return func(ctx context.Context, target string) (io.ReadCloser, error) {
		url, ok := urls[target]
		if !ok {
			return nil, nil
		}
		client := &http.Client{Timeout: 30 * time.Second}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("wtf: build request for %s: %w", target, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("wtf: fetch %s: %w", target, err)
		}
		if resp.StatusCode == http.StatusNotFound {
			_ = resp.Body.Close()
			return nil, nil
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("wtf: fetch %s: HTTP %d", url, resp.StatusCode)
		}
		return resp.Body, nil
	}
}

func (s *WtfSource) Name() string { return "wtf" }

// Drafts fetches each target README and maps every parseable gotcha entry to a
// quarantined trap draft, sorted by signature and capped at MaxWtfDrafts.
func (s *WtfSource) Drafts(ctx context.Context) ([]Draft, error) {
	var drafts []Draft
	for _, target := range s.targets {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		rc, err := s.fetch(ctx, target)
		if err != nil {
			return nil, err
		}
		if rc == nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(rc, 1<<22))
		_ = rc.Close()
		if readErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		switch target {
		case "wtfjs":
			drafts = append(drafts, parseWtfJS(string(body))...)
		case "wtfpython":
			drafts = append(drafts, parseWtfPython(string(body))...)
		}
	}
	sort.Slice(drafts, func(i, j int) bool {
		si, sj := drafts[i].Symptom.ErrorSignatures[0], drafts[j].Symptom.ErrorSignatures[0]
		if si != sj {
			return si < sj
		}
		return drafts[i].Title < drafts[j].Title
	})
	if len(drafts) > MaxWtfDrafts {
		drafts = drafts[:MaxWtfDrafts]
	}
	return drafts, nil
}

func parseWtfJS(body string) []Draft {
	body = wtfExamplesSlice(body)
	var drafts []Draft
	for title, section := range wtfSplitSections(body, "## ") {
		title = strings.TrimSpace(title)
		if wtfjsSkipTitle(title) {
			continue
		}
		snippet := wtfFirstFence(section, "js")
		explainM := wtfjsExplainRe.FindStringSubmatch(section)
		if snippet == "" || explainM == nil {
			continue
		}
		explanation := strings.TrimSpace(explainM[1])
		if explanation == "" {
			continue
		}
		oneLiner := wtfjsOneLiner(section, snippet)
		anchor := githubHeadingAnchor(title)
		drafts = append(drafts, wtfDraft(
			"wtfjs", title, oneLiner, snippet, explanation,
			fmt.Sprintf("%s#%s", wtfjsSourceBase, anchor),
			[]record.AppliesTo{{Ecosystem: "npm"}},
		))
	}
	return drafts
}

func parseWtfPython(body string) []Draft {
	body = wtfExamplesSlice(body)
	var drafts []Draft
	for title, section := range wtfSplitSections(body, "### ▶ ") {
		title = strings.TrimSpace(title)
		snippet := wtfPythonSnippet(section)
		explainM := wtfpythonExplainRe.FindStringSubmatch(section)
		if snippet == "" || explainM == nil {
			continue
		}
		explanation := strings.TrimSpace(explainM[1])
		if explanation == "" {
			continue
		}
		oneLiner := wtfpythonOneLiner(section, snippet)
		anchor := githubHeadingAnchor("▶ " + title)
		drafts = append(drafts, wtfDraft(
			"wtfpython", title, oneLiner, snippet, explanation,
			fmt.Sprintf("%s#%s", wtfpythonSourceBase, anchor),
			[]record.AppliesTo{{Ecosystem: "PyPI"}},
		))
	}
	return drafts
}

// wtfSplitSections splits markdown on a repeated heading prefix (e.g. "## ").
func wtfSplitSections(body, prefix string) map[string]string {
	sections := make(map[string]string)
	var curTitle string
	var b strings.Builder
	flush := func() {
		if curTitle != "" {
			sections[curTitle] = b.String()
		}
		b.Reset()
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, prefix) {
			flush()
			curTitle = strings.TrimPrefix(line, prefix)
			continue
		}
		if curTitle != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(line)
		}
	}
	flush()
	return sections
}

func wtfExamplesSlice(body string) string {
	const marker = "# 👀 Examples"
	if i := strings.Index(body, marker); i >= 0 {
		body = body[i+len(marker):]
	}
	if i := strings.Index(body, "\n# Contributing"); i >= 0 {
		body = body[:i]
	}
	if i := strings.Index(body, "\n# 📚 Other resources"); i >= 0 {
		body = body[:i]
	}
	return body
}

func wtfjsSkipTitle(title string) bool {
	switch title {
	case "👀 Examples", "Table of Contents":
		return true
	}
	return strings.HasPrefix(title, "Section:")
}

func wtfFirstFence(section, lang string) string {
	re := wtfFenceRe
	if lang == "js" {
		re = regexp.MustCompile("(?is)```(?:js|javascript)\\s*\n(.*?)```")
	}
	m := re.FindStringSubmatch(section)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func wtfPythonSnippet(section string) string {
	var parts []string
	for _, m := range wtfFenceRe.FindAllStringSubmatch(section, -1) {
		code := strings.TrimSpace(m[1])
		if code == "" {
			continue
		}
		parts = append(parts, code)
	}
	return strings.Join(parts, "\n\n")
}

func wtfjsOneLiner(section, snippet string) string {
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") {
			continue
		}
		if strings.Contains(line, "💡") {
			break
		}
		return strings.TrimRight(line, ":")
	}
	if m := wtfArrowResultRe.FindStringSubmatch(snippet); m != nil {
		first := strings.Split(strings.TrimSpace(snippet), "\n")[0]
		return strings.TrimSpace(first) + " evaluates to " + strings.TrimSpace(m[1])
	}
	first := strings.Split(strings.TrimSpace(snippet), "\n")[0]
	return strings.TrimSpace(first)
}

func wtfpythonOneLiner(section, snippet string) string {
	for _, line := range strings.Split(snippet, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, ">>>") {
			return strings.TrimPrefix(line, ">>> ")
		}
	}
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "```") &&
			!strings.HasPrefix(line, "**") && !strings.HasPrefix(line, "<!--") {
			return strings.TrimRight(line, ":")
		}
	}
	return strings.Split(strings.TrimSpace(snippet), "\n")[0]
}

func wtfDraft(collection, title, oneLiner, snippet, explanation, sourceURL string, applies []record.AppliesTo) Draft {
	sig := fmt.Sprintf("%s:%s", collection, githubHeadingAnchor(title))
	summary := title
	if oneLiner != "" {
		summary = title + ": " + oneLiner
	}
	rootCause := wtfRootCause(explanation)
	fix := wtfFixText(explanation)
	body := fmt.Sprintf("```\n%s\n```\n\n%s", snippet, strings.TrimSpace(explanation))
	return Draft{
		Kind:  "trap",
		Title: title,
		Symptom: &record.Symptom{
			Summary:         summary,
			ErrorSignatures: []string{sig},
		},
		AppliesTo: applies,
		Resolution: &record.Resolution{
			RootCause: rootCause,
			Fix:       fix,
		},
		Body:          body,
		SourceLicense: wtfSourceLicense,
		SourceURL:     sourceURL,
	}
}

func wtfRootCause(explanation string) string {
	for _, para := range strings.Split(explanation, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" || strings.HasPrefix(para, "```") || strings.HasPrefix(para, ">") {
			continue
		}
		if strings.HasPrefix(para, "- ") {
			return strings.TrimPrefix(para, "- ")
		}
		return para
	}
	return strings.TrimSpace(explanation)
}

func wtfFixText(explanation string) string {
	needles := []string{"avoid ", "don't ", "do not ", "never ", "instead ", "use ", "prefer ", "accept it", "delete the key"}
	for _, line := range strings.Split(explanation, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		for _, needle := range needles {
			if strings.Contains(lower, needle) {
				return strings.TrimPrefix(line, "- ")
			}
		}
	}
	for _, line := range strings.Split(explanation, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			return strings.TrimPrefix(line, "- ")
		}
	}
	return wtfRootCause(explanation)
}

// githubHeadingAnchor matches GitHub's README heading anchor slug algorithm.
func githubHeadingAnchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' {
			b.WriteRune(r)
		}
	}
	return strings.ReplaceAll(b.String(), " ", "-")
}
