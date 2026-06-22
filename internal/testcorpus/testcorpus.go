// SPDX-License-Identifier: AGPL-3.0-only

// Package testcorpus provides a frozen, small fixture corpus for engine-logic
// tests. It avoids loading the live experience/ corpus (ADR-0021 phase 1).
package testcorpus

import (
	"path/filepath"
	"runtime"
)

// Root returns the absolute path to the bundled fixture corpus root.
// Pass this to record.LoadCorpus instead of "../..".
func Root() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "corpus")
}
