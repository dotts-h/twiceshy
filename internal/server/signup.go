// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// signup issuance defaults for a self-serve alpha token (#0127).
const (
	signupDailyQuota     = 1000
	signupRatePerMin     = 60
	signupMaxPerIPPerDay = 3
	// maxSignupBodyBytes bounds the request body: /signup bypasses the shared
	// withMaxBytes middleware (it sits on the outer, unauthenticated mux, next
	// to /healthz), so it caps its own body — a tiny JSON payload never needs more.
	maxSignupBodyBytes = 4 << 10
)

// TokenIssuer mints new tenant tokens for POST /signup (#0127). Its method set
// matches *index.Index; kept separate from TokenStore (auth/quota reads)
// because issuance is a write, the only one a public endpoint performs.
type TokenIssuer interface {
	IssueToken(label string, dailyQuota, ratePerMin int, now time.Time) (full string, id string, err error)
}

// SignupArgs is the POST /signup request body: a public self-serve request for
// an alpha tenant token. No email verification in the alpha; accepting the
// terms is enforced server-side, not just by the landing-page UI.
type SignupArgs struct {
	Email       string `json:"email"`
	AcceptTerms bool   `json:"accept_terms"`
}

// SignupResult is the POST /signup response: the full bearer token, shown once
// and never retrievable again — the store only ever holds its hash.
type SignupResult struct {
	Token       string `json:"token"`
	QuotaPerDay int    `json:"quota_per_day"`
	RatePerMin  int    `json:"rate_per_min"`
}

// signupIPLimiter is an in-memory per-IP daily counter for POST /signup
// (#0127): a coarser, IP-keyed guard on top of token issuance, independent of
// the global request-rate bucket /signup also sits behind. Reset by UTC day,
// like token daily quotas.
type signupIPLimiter struct {
	mu     sync.Mutex
	day    string
	counts map[string]int
	now    func() time.Time
}

func newSignupIPLimiter(now func() time.Time) *signupIPLimiter {
	return &signupIPLimiter{counts: make(map[string]int), now: now}
}

// allow reports whether ip is still under signupMaxPerIPPerDay for today (UTC),
// counting this call if so.
func (l *signupIPLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	day := l.now().UTC().Format("2006-01-02")
	if day != l.day {
		l.day = day
		l.counts = make(map[string]int)
	}
	if l.counts[ip] >= signupMaxPerIPPerDay {
		return false
	}
	l.counts[ip]++
	return true
}

// signupHTTP issues a self-serve alpha tenant token (#0127). Public: it
// bypasses tenantAuth like /healthz, but sits behind the global rate limiter
// (wired by New) and this handler's own per-IP daily cap. When signup is
// disabled (the default), it answers 404 — indistinguishable from a route that
// was never wired, rather than a 503 that would reveal the feature exists.
func (h *handlers) signupHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.signupEnabled {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := resolveSignupClientIP(r, h.trustedProxies)
	if !h.signupLimiter.allow(ip) {
		w.Header().Set("Retry-After", strconv.Itoa(secondsUntilUTCMidnight(time.Now())))
		writeSignupError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSignupBodyBytes)
	var args SignupArgs
	if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
		writeSignupError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if !args.AcceptTerms {
		writeSignupError(w, http.StatusBadRequest, "terms_not_accepted")
		return
	}
	email, ok := normalizeSignupEmail(args.Email)
	if !ok {
		writeSignupError(w, http.StatusBadRequest, "invalid_email")
		return
	}

	full, id, err := h.signupIssuer.IssueToken(email, signupDailyQuota, signupRatePerMin, time.Now())
	if err != nil {
		h.logger.Error("signup: issue token failed", slog.String("error", err.Error()))
		writeSignupError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	// Log the signup for audit — email, token id, ip — but never the secret: the
	// full token is only ever in the HTTP response body, never in a log line.
	h.logger.Info("signup",
		slog.String("email", email),
		slog.String("token_id", id),
		slog.String("remote_addr", ip),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(SignupResult{Token: full, QuotaPerDay: signupDailyQuota, RatePerMin: signupRatePerMin})
}

func writeSignupError(w http.ResponseWriter, status int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":%q}`+"\n", reason)
}

// signupClientIP returns the caller's address without the port, for the per-IP
// signup limiter — RemoteAddr always carries a port on a real listener.
func signupClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// resolveSignupClientIP is signupClientIP, made trusted-proxy aware (#0131
// finding 3): behind a reverse proxy, RemoteAddr is always the proxy's own
// address, so the per-IP signup cap effectively capped the whole deployment
// at signupMaxPerIPPerDay instead of one caller. X-Forwarded-For is only
// honored when RemoteAddr itself matches a configured trusted proxy — an
// untrusted caller can set that header to anything, so trusting it
// unconditionally would let a spoofed value bypass the cap entirely. When
// trusted, the LAST XFF entry of the LAST header LINE is used: a client may
// send its own X-Forwarded-For line and a proxy may append its value as a
// SEPARATE header line rather than merging into one comma list — Header.Get
// returns only that first, attacker-controlled line. The last entry of the
// last line is the one the trusted proxy appended (the real client);
// everything earlier is client-supplied and spoofable. Any parse failure
// (unset trustedProxies, RemoteAddr not trusted, missing/unparsable XFF)
// falls back to RemoteAddr.
func resolveSignupClientIP(r *http.Request, trustedProxies []*net.IPNet) string {
	remoteHost := signupClientIP(r)
	if len(trustedProxies) == 0 {
		return remoteHost
	}
	remoteIP := net.ParseIP(remoteHost)
	if remoteIP == nil || !ipTrusted(remoteIP, trustedProxies) {
		return remoteHost
	}
	lines := r.Header.Values("X-Forwarded-For")
	if len(lines) == 0 {
		return remoteHost
	}
	parts := strings.Split(lines[len(lines)-1], ",")
	last := strings.TrimSpace(parts[len(parts)-1])
	if net.ParseIP(last) == nil {
		return remoteHost
	}
	return last
}

func ipTrusted(ip net.IP, trustedProxies []*net.IPNet) bool {
	for _, n := range trustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ParseTrustedProxies parses TWICESHY_TRUSTED_PROXIES: a comma-separated list
// of CIDRs for reverse proxies allowed to set X-Forwarded-For on POST /signup
// (#0131). A bare IP (no "/") is accepted as an exact match — /32 for IPv4,
// /128 for IPv6. Empty input returns (nil, nil): no trusted proxies, the
// current RemoteAddr-only behavior. Any invalid entry is an error, by design —
// the deployment should fail fast at startup on a typo, not silently trust
// nothing (or, worse, the wrong network).
func ParseTrustedProxies(raw string) ([]*net.IPNet, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	entries := strings.Split(raw, ",")
	out := make([]*net.IPNet, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		cidr := entry
		if !strings.Contains(cidr, "/") {
			ip := net.ParseIP(entry)
			if ip == nil {
				return nil, fmt.Errorf("trusted proxies: invalid address %q", entry)
			}
			if ip.To4() != nil {
				cidr += "/32"
			} else {
				cidr += "/128"
			}
		}
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("trusted proxies: invalid CIDR %q: %w", entry, err)
		}
		out = append(out, ipNet)
	}
	return out, nil
}

// normalizeSignupEmail trims and lowercases raw, then validates it: 3..254
// bytes, exactly one "@" with a non-empty local part and domain, and a "."
// somewhere in the domain. No deliverability check — the alpha sends no
// verification email.
func normalizeSignupEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if len(email) < 3 || len(email) > 254 {
		return "", false
	}
	if strings.Count(email, "@") != 1 {
		return "", false
	}
	at := strings.IndexByte(email, '@')
	local, domain := email[:at], email[at+1:]
	if local == "" || domain == "" || !strings.Contains(domain, ".") {
		return "", false
	}
	return email, true
}
