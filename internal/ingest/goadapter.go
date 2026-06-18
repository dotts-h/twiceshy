// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import _ "embed"

// goDeprecationsYAML is the curated, license-clean Go stdlib deprecation set
// (staticcheck SA1019 diagnostics as the fingerprintable signatures).
// Distilled facts only (ADR-0003 §4).
//
//go:embed data/go-deprecations.yaml
var goDeprecationsYAML []byte

// NewGoSource returns the Go stdlib deprecation importer source.
func NewGoSource() Source {
	return deprecationSource{name: "go", ecosystem: "Go", data: goDeprecationsYAML}
}
