// SPDX-License-Identifier: AGPL-3.0-only

package agenteval

import (
	"strings"
	"testing"
)

func TestGoldTasks_WellFormed(t *testing.T) {
	cases := GoldTasks()
	if len(cases) != 3 {
		t.Fatalf("GoldTasks() len = %d, want 3", len(cases))
	}
	seen := make(map[string]bool, len(cases))
	for _, c := range cases {
		if strings.TrimSpace(c.TrapID) == "" {
			t.Error("empty TrapID")
		}
		if strings.TrimSpace(c.Prompt) == "" {
			t.Errorf("%s: empty Prompt", c.TrapID)
		}
		if strings.TrimSpace(c.Card) == "" {
			t.Errorf("%s: empty Card", c.TrapID)
		}
		if strings.TrimSpace(c.VerifyID) == "" {
			t.Errorf("%s: empty VerifyID", c.TrapID)
		}
		if seen[c.TrapID] {
			t.Errorf("duplicate TrapID %q", c.TrapID)
		}
		seen[c.TrapID] = true
	}
}
