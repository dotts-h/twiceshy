// SPDX-License-Identifier: AGPL-3.0-only

package ingest

import "context"

// Source is a corpus-importer adapter (#0007): it turns a license-clean
// knowledge source into quarantined-record Drafts. Adapters are pure — they
// emit Drafts and nothing else; the CLI edge (twiceshy ingest) dedups,
// allocates ids, and persists. Everything imported is born quarantined and is
// pull-only until Doctor 3 can run a guard (ADR-0003 §5).
type Source interface {
	// Name is the source's CLI selector (e.g. "go").
	Name() string
	// Drafts returns the curated drafts this source contributes.
	Drafts(ctx context.Context) ([]Draft, error)
}
