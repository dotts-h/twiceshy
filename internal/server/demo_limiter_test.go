// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"fmt"
	"testing"
	"time"
)

func TestDemoLimiterGlobalCap(t *testing.T) {
	fixed := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	l := newDemoLimiter(func() time.Time { return fixed })

	for i := 0; i < demoGlobalPerDay; i++ {
		ok, reason := l.allow(fmt.Sprintf("ip-%d", i))
		if !ok {
			t.Fatalf("request %d: allow = false, reason = %q", i+1, reason)
		}
	}
	ok, reason := l.allow("ip-extra")
	if ok {
		t.Fatal("501st allow = true, want false")
	}
	if reason != "global_limit_exceeded" {
		t.Fatalf("reason = %q, want global_limit_exceeded", reason)
	}
}
