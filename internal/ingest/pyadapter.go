// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import _ "embed"

// pyDeprecationsYAML is the curated, license-clean Python version-breaking set
// (the GitChameleon problem class; runtime AttributeError/DeprecationWarning as
// the fingerprintable signatures). Distilled facts only (ADR-0003 §4).
//
//go:embed data/py-deprecations.yaml
var pyDeprecationsYAML []byte

// NewPySource returns the Python version-breaking importer source.
func NewPySource() Source {
	return deprecationSource{name: "py", ecosystem: "PyPI", data: pyDeprecationsYAML}
}
