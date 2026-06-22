// SPDX-License-Identifier: AGPL-3.0-only

package screen_test

import (
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/screen"
)

func TestHasSecret(t *testing.T) {
	// Secret-shaped value assembled at run time, never a literal token in any commit
	// (CONVENTIONS: gitleaks scans the whole history).
	secret := "AKIA" + strings.Repeat("A", 16)

	if !screen.HasSecret(screen.Scan("key " + secret)) {
		t.Error("HasSecret = false for an AWS-key-shaped secret, want true")
	}
	if screen.HasSecret(screen.Scan("ran the build on 192.168.50.244")) {
		t.Error("HasSecret = true for a pii-only finding (private IP), want false (only secrets block)")
	}
	if screen.HasSecret(nil) {
		t.Error("HasSecret = true for no findings, want false")
	}
}
