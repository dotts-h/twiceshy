// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"testing"

	"github.com/dotts-h/twiceshy/internal/record"
)

func TestUsageEqual_AllBranches(t *testing.T) {
	hit := func(s string) *string { return &s }
	cases := []struct {
		name string
		a, b *record.Usage
		want bool
	}{
		{"both nil", nil, nil, true},
		{"a nil b set", nil, &record.Usage{}, false},
		{"a set b nil", &record.Usage{}, nil, false},
		{"retrieved differ", &record.Usage{Retrieved: 1}, &record.Usage{Retrieved: 2}, false},
		{"confirmed differ", &record.Usage{ConfirmedHelpful: 1}, &record.Usage{ConfirmedHelpful: 2}, false},
		{"lasthit both nil", &record.Usage{Retrieved: 1}, &record.Usage{Retrieved: 1}, true},
		{"lasthit a nil b set", &record.Usage{}, &record.Usage{LastHit: hit("2026-06-19")}, false},
		{"lasthit a set b nil", &record.Usage{LastHit: hit("2026-06-19")}, &record.Usage{}, false},
		{"lasthit differ", &record.Usage{LastHit: hit("2026-06-18")}, &record.Usage{LastHit: hit("2026-06-19")}, false},
		{"lasthit equal", &record.Usage{LastHit: hit("2026-06-19")}, &record.Usage{LastHit: hit("2026-06-19")}, true},
		{"fully equal", &record.Usage{Retrieved: 3, ConfirmedHelpful: 1, LastHit: hit("2026-06-19")}, &record.Usage{Retrieved: 3, ConfirmedHelpful: 1, LastHit: hit("2026-06-19")}, true},
	}
	for _, tc := range cases {
		if got := usageEqual(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: usageEqual = %v, want %v", tc.name, got, tc.want)
		}
	}
}
