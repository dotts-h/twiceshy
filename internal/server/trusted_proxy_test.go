// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestResolveSignupClientIP is #0131 finding 3: signupClientIP always trusted
// RemoteAddr, so a deployment behind a reverse proxy saw the proxy's own IP for
// every signup — the per-IP daily cap effectively capped the whole service at
// signupMaxPerIPPerDay, not one caller. resolveSignupClientIP only trusts
// X-Forwarded-For when RemoteAddr itself is a configured trusted proxy, and
// always takes the LAST entry (the one that trusted proxy appended — earlier
// entries are client-supplied and spoofable).
func TestResolveSignupClientIP(t *testing.T) {
	trustedV4, err := ParseTrustedProxies("10.0.0.0/8")
	if err != nil {
		t.Fatalf("ParseTrustedProxies: %v", err)
	}
	trustedV6, err := ParseTrustedProxies("::1/128")
	if err != nil {
		t.Fatalf("ParseTrustedProxies: %v", err)
	}

	cases := []struct {
		name       string
		remoteAddr string
		xff        string
		trusted    []*net.IPNet
		want       string
	}{
		{
			name:       "no trusted proxies configured falls back to RemoteAddr",
			remoteAddr: "1.2.3.4:5555",
			xff:        "9.9.9.9",
			trusted:    nil,
			want:       "1.2.3.4",
		},
		{
			name:       "trusted proxy + single XFF entry is used",
			remoteAddr: "10.0.0.1:5555",
			xff:        "1.2.3.4",
			trusted:    trustedV4,
			want:       "1.2.3.4",
		},
		{
			name:       "trusted proxy + multi-hop XFF uses the LAST entry",
			remoteAddr: "10.0.0.1:5555",
			xff:        "9.9.9.9, 1.2.3.4",
			trusted:    trustedV4,
			want:       "1.2.3.4",
		},
		{
			name:       "untrusted RemoteAddr ignores XFF entirely (spoof attempt)",
			remoteAddr: "8.8.8.8:5555",
			xff:        "1.2.3.4",
			trusted:    trustedV4,
			want:       "8.8.8.8",
		},
		{
			name:       "trusted proxy + garbage XFF entry falls back to RemoteAddr",
			remoteAddr: "10.0.0.1:5555",
			xff:        "not-an-ip",
			trusted:    trustedV4,
			want:       "10.0.0.1",
		},
		{
			name:       "IPv6 remote + IPv6 XFF works",
			remoteAddr: "[::1]:5555",
			xff:        "2001:db8::1",
			trusted:    trustedV6,
			want:       "2001:db8::1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://example/signup", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			got := resolveSignupClientIP(req, tc.trusted)
			if got != tc.want {
				t.Fatalf("resolveSignupClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseTrustedProxies(t *testing.T) {
	t.Run("empty is nil, no error", func(t *testing.T) {
		got, err := ParseTrustedProxies("")
		if err != nil || got != nil {
			t.Fatalf("ParseTrustedProxies(\"\") = %v, %v, want nil, nil", got, err)
		}
	})

	t.Run("single IP accepted as /32", func(t *testing.T) {
		nets, err := ParseTrustedProxies("192.168.1.1")
		if err != nil {
			t.Fatalf("ParseTrustedProxies: %v", err)
		}
		if len(nets) != 1 || !nets[0].Contains(net.ParseIP("192.168.1.1")) || nets[0].Contains(net.ParseIP("192.168.1.2")) {
			t.Fatalf("192.168.1.1 must parse as an exact /32, got %v", nets)
		}
	})

	t.Run("single IPv6 accepted as /128", func(t *testing.T) {
		nets, err := ParseTrustedProxies("::1")
		if err != nil {
			t.Fatalf("ParseTrustedProxies: %v", err)
		}
		if len(nets) != 1 || !nets[0].Contains(net.ParseIP("::1")) || nets[0].Contains(net.ParseIP("::2")) {
			t.Fatalf("::1 must parse as an exact /128, got %v", nets)
		}
	})

	t.Run("comma-separated CIDRs and IPs mix", func(t *testing.T) {
		nets, err := ParseTrustedProxies("10.0.0.0/8, 192.168.1.1")
		if err != nil {
			t.Fatalf("ParseTrustedProxies: %v", err)
		}
		if len(nets) != 2 {
			t.Fatalf("len(nets) = %d, want 2", len(nets))
		}
	})

	t.Run("invalid entry fails fast", func(t *testing.T) {
		_, err := ParseTrustedProxies("not-a-cidr")
		if err == nil {
			t.Fatal("expected an error for an invalid trusted-proxy entry")
		}
		if !strings.Contains(err.Error(), "not-a-cidr") {
			t.Fatalf("error = %q, want it to name the bad entry", err.Error())
		}
	})
}
