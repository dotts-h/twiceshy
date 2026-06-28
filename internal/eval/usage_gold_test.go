// SPDX-License-Identifier: AGPL-3.0-only

package eval

import (
	"strings"
	"testing"
)

func TestUsageGold_WellFormed(t *testing.T) {
	cases := UsageGold()
	if len(cases) != 3 {
		t.Fatalf("UsageGold() len = %d, want 3", len(cases))
	}
	for _, c := range cases {
		if strings.TrimSpace(c.Transcript) == "" {
			t.Errorf("%s: empty transcript", c.Name)
		}
		served := make(map[string]bool, len(c.Served))
		for _, id := range c.Served {
			served[id] = true
		}
		for _, id := range c.Used {
			if !served[id] {
				t.Errorf("%s: used id %q not in served %v", c.Name, id, c.Served)
			}
		}
	}
}
