// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"errors"
	"testing"
)

// exitCode maps the anomaly halt to a process code distinct from a usage error
// or a generic failure, so an unattended wrapper can react to it specifically.
func TestExitCode_AnomalyHaltIsDistinct(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"usage", errUsage, 2},
		{"anomaly", errAnomalyHalt, 3},
		{"wrapped anomaly", errors.Join(errors.New("ctx"), errAnomalyHalt), 3},
		{"other", errors.New("broker exploded"), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := exitCode(c.err); got != c.want {
				t.Errorf("exitCode = %d, want %d", got, c.want)
			}
		})
	}
}
