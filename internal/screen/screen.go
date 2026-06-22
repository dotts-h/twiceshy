// SPDX-License-Identifier: AGPL-3.0-only

// Package screen is the ingestion safety gate (#0011, SECURITY_ANALYSIS.md
// Facet 2): it scans candidate experience-record text for content that must
// never be silently ingested — secrets, executable harmful-code sequences, and
// PII. It is a pure, deterministic detector; the caller (ingest.Prepare) owns
// the policy (quarantine-with-flag by default, or reject). Findings never echo a
// raw secret — the Redacted field is masked.
package screen

import (
	"math"
	"regexp"
	"sort"
)

// Finding is one detected hazard. Category is secret | harmful-code | pii; Rule
// is a short stable id; Redacted is a masked snippet (never the raw match).
type Finding struct {
	Category string
	Rule     string
	Redacted string
}

type sigRule struct {
	category string
	name     string
	re       *regexp.Regexp
}

// patternRules are precise, single-shot detectors. Harmful-code rules match only
// EXECUTABLE sequences (pipe-into-interpreter, reverse shells, fork bomb,
// root rm) — never bare URLs or attack prose — so advisory records (which
// describe attacks, e.g. "${jndi:ldap://...}") do not false-positive.
var patternRules = []sigRule{
	// secrets — known token shapes
	{"secret", "aws-access-key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"secret", "github-token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`)},
	{"secret", "github-pat", regexp.MustCompile(`github_pat_[A-Za-z0-9_]{22,}`)},
	{"secret", "google-api-key", regexp.MustCompile(`AIza[0-9A-Za-z_\-]{35}`)},
	{"secret", "slack-token", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)},
	{"secret", "jwt", regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)},
	{"secret", "private-key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA |PGP )?PRIVATE KEY-----`)},
	// harmful-code — executable sequences only
	{"harmful-code", "pipe-to-shell", regexp.MustCompile(`(?i)\b(?:curl|wget)\b[^\n|]*\|\s*(?:sudo\s+)?(?:ba)?sh\b`)},
	{"harmful-code", "base64-pipe-shell", regexp.MustCompile(`(?i)base64\s+-d[^\n|]*\|\s*(?:ba)?sh\b`)},
	{"harmful-code", "reverse-shell-devtcp", regexp.MustCompile(`/dev/tcp/`)},
	{"harmful-code", "netcat-exec", regexp.MustCompile(`(?i)\bnc\b[^\n]*\s-e\s`)},
	{"harmful-code", "rm-rf-root", regexp.MustCompile(`\brm\s+-rf\s+/(?:\s|$)`)},
	{"harmful-code", "fork-bomb", regexp.MustCompile(`:\(\)\s*\{[^}]*:\|:[^}]*\}\s*;\s*:`)},
	// pii — conservative (RFC-1918 private ranges only, so version numbers don't
	// match). Loopback (127.0.0.0/8) is localhost, not PII — and Docker's embedded
	// DNS at 127.0.0.11 appears legitimately in sandbox/repro scripts (exp-0016),
	// so it is deliberately excluded.
	{"pii", "email", regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)},
	{"pii", "private-ip", regexp.MustCompile(`\b10(?:\.\d{1,3}){3}\b|\b192\.168(?:\.\d{1,3}){2}\b`)},
}

// reAssignedSecret catches a generic high-entropy value assigned to a
// secret-shaped key. Gated on the key word + length so prose never trips it; the
// captured value is entropy-checked before flagging.
var reAssignedSecret = regexp.MustCompile(`(?i)(?:api[_-]?key|secret|passwd|password|token)\s*[:=]\s*['"]?([A-Za-z0-9+/=_\-]{20,})`)

// entropyThreshold (bits/char) above which an assigned value is treated as a
// random secret rather than a word/phrase.
const entropyThreshold = 3.5

// Scan returns the deduped, sorted hazards found across the given texts. A given
// (category, rule) is reported at most once regardless of how many texts hit it.
func Scan(texts ...string) []Finding {
	seen := map[string]bool{}
	var out []Finding
	add := func(f Finding) {
		k := f.Category + ":" + f.Rule
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, f)
	}
	for _, t := range texts {
		for _, r := range patternRules {
			if m := r.re.FindString(t); m != "" {
				add(Finding{r.category, r.name, mask(m)})
			}
		}
		for _, m := range reAssignedSecret.FindAllStringSubmatch(t, -1) {
			if shannon(m[1]) >= entropyThreshold {
				add(Finding{"secret", "assigned-high-entropy", mask(m[1])})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Rule < out[j].Rule
	})
	return out
}

// ExecutionHazards returns the subset of findings that make a script unsafe to
// EXECUTE — embedded secrets and harmful-code sequences. PII findings are
// excluded on purpose: a repro/test fixture may legitimately contain an email or
// a private IP, and PII is an ingestion/storage concern, not an execution one
// (the broker's execute phase has no network to exfiltrate anything). This is the
// calibrated gate the sandbox broker (#0018/#0019) uses to refuse a repro.
func ExecutionHazards(findings []Finding) []Finding {
	out := make([]Finding, 0, len(findings))
	for _, f := range findings {
		if f.Category == "secret" || f.Category == "harmful-code" {
			out = append(out, f)
		}
	}
	return out
}

// HasSecret reports whether any finding is a secret. This is the single home of the
// fail-closed gate the /retro endpoint and the `twiceshy screen` CLI both use to
// refuse a transcript before it leaves the machine — harmful-code / pii are expected
// in a coding transcript (shell snippets, private IPs) and do not block.
func HasSecret(findings []Finding) bool {
	for _, f := range findings {
		if f.Category == "secret" {
			return true
		}
	}
	return false
}

// Flags renders findings as deduped, sorted "category:rule" tags for storage in
// provenance.security_flags. It carries no secret material.
func Flags(fs []Finding) []string {
	flags := make([]string, 0, len(fs))
	for _, f := range fs {
		flags = append(flags, f.Category+":"+f.Rule)
	}
	sort.Strings(flags)
	return flags
}

// mask hides the body of a match so a Finding never reveals a usable secret.
func mask(s string) string {
	r := []rune(s)
	if len(r) <= 8 {
		return "****"
	}
	return string(r[:3]) + "…" + string(r[len(r)-2:])
}

// shannon is the Shannon entropy (bits per character) of s.
func shannon(s string) float64 {
	if s == "" {
		return 0
	}
	freq := map[rune]float64{}
	for _, r := range s {
		freq[r]++
	}
	n := float64(len([]rune(s)))
	var h float64
	for _, c := range freq {
		p := c / n
		h -= p * math.Log2(p)
	}
	return h
}
