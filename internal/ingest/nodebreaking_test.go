// SPDX-License-Identifier: AGPL-3.0-only

package ingest_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/dotts-h/twiceshy/internal/index"
	"github.com/dotts-h/twiceshy/internal/ingest"
	"github.com/dotts-h/twiceshy/internal/record"
)

// nodeChangelogFixtureV22 is a frozen excerpt of the real shape of
// https://raw.githubusercontent.com/nodejs/node/main/doc/changelogs/CHANGELOG_V22.md
// (MIT), downloaded once while developing this adapter. It carries two release
// sections with SEMVER-MAJOR commit lines mixed with ordinary (non-major) commit
// lines and CVE-tagged lines (noise), plus one malformed SEMVER-MAJOR line (the
// rare `_**Revert**_ "**subsystem**: ..."` shape) that must be skipped, not fatal.
const nodeChangelogFixtureV22 = `# Node.js 22 ChangeLog

<a id="22.23.0"></a>

## 2026-06-18, Version 22.23.0 'Jod' (LTS), @aduh95

This is a security release.

### Notable Changes

* (CVE-2026-48618) tls: normalize hostname for server identity checks (Matteo Collina) – High

### Commits

* \[[` + "`38b4c5ed51`" + `](https://github.com/nodejs/node/commit/38b4c5ed51)] - **(CVE-2026-48933)** **crypto**: guard WebCrypto cipher output length (Filip Skokan) [nodejs-private/node-private#878](https://github.com/nodejs-private/node-private/pull/878)
* \[[` + "`0f48583512`" + `](https://github.com/nodejs/node/commit/0f48583512)] - **(SEMVER-MAJOR)** **deps**: update nghttp2 to 1.69.0 (Node.js GitHub Bot) [#62891](https://github.com/nodejs/node/pull/62891)
* \[[` + "`0c37bff2ff`" + `](https://github.com/nodejs/node/commit/0c37bff2ff)] - **http2**: fix DEP0194 message (KaKa) [#58669](https://github.com/nodejs/node/pull/58669)
* \[[` + "`ea5dc6b529`" + `](https://github.com/nodejs/node/commit/ea5dc6b529)] - **(SEMVER-MAJOR)** **http2**: remove support for priority signaling (Matteo Collina) [#58293](https://github.com/nodejs/node/pull/58293)

<a id="22.0.0"></a>

## 2024-04-24, Version 22.0.0 (Current), @RafaelGSS and @marco-ippolito

We're excited to announce the release of Node.js 22!

### Other Notable Changes

* \[[` + "`818c10e86d`" + `](https://github.com/nodejs/node/commit/818c10e86d)] - **lib**: improve perf of ` + "`AbortSignal`" + ` creation (Raz Luvaton) [#52408](https://github.com/nodejs/node/pull/52408)
* \[[` + "`c975384264`" + `](https://github.com/nodejs/node/commit/c975384264)] - **(SEMVER-MAJOR)** **lib**: enable WebSocket by default (Aras Abbasi) [#51594](https://github.com/nodejs/node/pull/51594)
* \[[` + "`60e836427e`" + `](https://github.com/nodejs/node/commit/60e836427e)] - **(SEMVER-MAJOR)** **console**: treat non-strings as separate argument in console.assert() (Jacob Hummer) [#49722](https://github.com/nodejs/node/pull/49722)
* \[[` + "`c7493fac5e`" + `](https://github.com/nodejs/node/commit/c7493fac5e)] - **(SEMVER-MAJOR)** _**Revert**_ "**test**: disable fast API call count checks" (Michaël Zasso) [#58070](https://github.com/nodejs/node/pull/58070)
`

func stubNodeBreaking(bodies map[string]string) ingest.NodeBreakingOption {
	return ingest.WithNodeBreakingFetch(func(_ context.Context, target string) (io.ReadCloser, error) {
		body, ok := bodies[target]
		if !ok {
			return nil, nil // 404 -> skip
		}
		return io.NopCloser(strings.NewReader(body)), nil
	})
}

func nodeBreakingSigs(drafts []ingest.Draft) []string {
	var out []string
	for _, d := range drafts {
		if d.Symptom != nil && len(d.Symptom.ErrorSignatures) > 0 {
			out = append(out, d.Symptom.ErrorSignatures[0])
		}
	}
	return out
}

func TestNodeBreaking_Name(t *testing.T) {
	if got := ingest.NewNodeBreakingSource().Name(); got != "node-breaking" {
		t.Errorf("Name = %q, want node-breaking", got)
	}
}

// SEMVER-MAJOR commit lines map to trap drafts with the documented fields; the
// malformed Revert-shaped SEMVER-MAJOR line is skipped, never fatal; ordinary and
// CVE-tagged commit lines (not SEMVER-MAJOR) never yield drafts.
func TestNodeBreaking_ExtractsSemverMajorEntries(t *testing.T) {
	src := ingest.NewNodeBreakingSource(
		ingest.WithNodeBreakingTargets([]string{"V22"}),
		stubNodeBreaking(map[string]string{"V22": nodeChangelogFixtureV22}),
	)
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	// 3 real SEMVER-MAJOR lines (deps, http2, lib) yield drafts; console's subject
	// contains its own parens and must still parse; the malformed Revert-shaped
	// SEMVER-MAJOR line (test) is skipped.
	if len(drafts) != 4 {
		t.Fatalf("want 4 drafts (deps, http2, lib, console), got %d: %+v", len(drafts), drafts)
	}

	byPkg := map[string]ingest.Draft{}
	for _, d := range drafts {
		byPkg[d.AppliesTo[0].Runtime["node"]+":"+d.Symptom.ErrorSignatures[0]] = d
	}

	deps, ok := byPkg["22.23.0:node:breaking:22.23.0:deps"]
	if !ok {
		t.Fatalf("missing deps draft: %+v", drafts)
	}
	if deps.Kind != "trap" {
		t.Errorf("kind = %q, want trap", deps.Kind)
	}
	if deps.Title != "Node.js 22.23.0: deps: update nghttp2 to 1.69.0 is a breaking change" {
		t.Errorf("title = %q", deps.Title)
	}
	if !strings.Contains(deps.Symptom.Summary, "22.23.0") || !strings.Contains(deps.Symptom.Summary, "deps") {
		t.Errorf("summary = %q", deps.Symptom.Summary)
	}
	if deps.Resolution == nil || !strings.Contains(deps.Resolution.RootCause, "SEMVER-MAJOR") {
		t.Errorf("root_cause = %+v", deps.Resolution)
	}
	if deps.Resolution == nil || !strings.Contains(deps.Resolution.Fix, "https://github.com/nodejs/node/pull/62891") {
		t.Errorf("fix should link the PR: %+v", deps.Resolution)
	}
	if deps.SourceLicense != record.SourceLicenseFactsOnly {
		t.Errorf("source_license = %q, want facts-only", deps.SourceLicense)
	}
	wantURL := "https://raw.githubusercontent.com/nodejs/node/main/doc/changelogs/CHANGELOG_V22.md#22.23.0"
	if deps.SourceURL != wantURL {
		t.Errorf("source_url = %q, want %q", deps.SourceURL, wantURL)
	}

	console, ok := byPkg["22.0.0:node:breaking:22.0.0:console"]
	if !ok {
		t.Fatalf("missing console draft (subject with its own parens must still parse): %+v", drafts)
	}
	if !strings.Contains(console.Title, "console.assert()") {
		t.Errorf("console title lost its parenthesized subject: %q", console.Title)
	}

	// Facts-only (ADR-0003 §4): no changelog prose beyond the short factual
	// subject line is reproduced (e.g. the security-release paragraph, or the
	// "excited to announce" marketing prose, must never leak into a draft).
	for _, d := range drafts {
		all := d.Title + d.Symptom.Summary + d.Resolution.RootCause + d.Resolution.Fix + d.Body
		if strings.Contains(all, "excited to announce") || strings.Contains(all, "unexpected behavior") {
			t.Errorf("draft reproduced changelog prose beyond the factual subject line: %+v", d)
		}
	}
}

// An unknown/retired major (production 404 -> nil reader) is skipped, not an
// error; a known target alongside it still yields its drafts.
func TestNodeBreaking_SkipsUnknownMajor(t *testing.T) {
	src := ingest.NewNodeBreakingSource(
		ingest.WithNodeBreakingTargets([]string{"V999", "V22"}),
		stubNodeBreaking(map[string]string{"V22": nodeChangelogFixtureV22}),
	)
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 4 {
		t.Fatalf("want 4 drafts from V22 alone, got %d", len(drafts))
	}
}

// A fetch/HTTP error for one target is systemic and fails the batch (parity with
// npmlive/eollive: a 404 is a skip, but any other failure is not swallowed).
func TestNodeBreaking_FetchErrorFailsBatch(t *testing.T) {
	wantErr := errors.New("boom")
	src := ingest.NewNodeBreakingSource(
		ingest.WithNodeBreakingTargets([]string{"V22"}),
		ingest.WithNodeBreakingFetch(func(_ context.Context, _ string) (io.ReadCloser, error) {
			return nil, wantErr
		}),
	)
	if _, err := src.Drafts(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
}

// Drafts is deterministic: the same fixture yields byte-identical signatures
// across calls (the precondition for idempotent re-imports under the dedup
// layer), sorted by version then subsystem.
func TestNodeBreaking_DeterministicOrder(t *testing.T) {
	mk := func() []string {
		src := ingest.NewNodeBreakingSource(
			ingest.WithNodeBreakingTargets([]string{"V22"}),
			stubNodeBreaking(map[string]string{"V22": nodeChangelogFixtureV22}),
		)
		d, err := src.Drafts(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return nodeBreakingSigs(d)
	}
	a, b := mk(), mk()
	if strings.Join(a, ",") != strings.Join(b, ",") {
		t.Fatalf("non-deterministic: %v vs %v", a, b)
	}
	want := "node:breaking:22.0.0:console,node:breaking:22.0.0:lib,node:breaking:22.23.0:deps,node:breaking:22.23.0:http2"
	if strings.Join(a, ",") != want {
		t.Fatalf("order = %v, want %s", a, want)
	}
}

// A context cancelled mid-read is systemic and must fail loud, not be swallowed
// as a per-target skip (parity with eollive/npmlive).
func TestNodeBreaking_DraftsPropagatesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fetch := func(_ context.Context, _ string) (io.ReadCloser, error) {
		return io.NopCloser(&cancelDuringReadReader{cancel: cancel}), nil
	}
	src := ingest.NewNodeBreakingSource(
		ingest.WithNodeBreakingTargets([]string{"V22"}),
		ingest.WithNodeBreakingFetch(fetch),
	)
	if _, err := src.Drafts(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// The per-run cap (MaxNodeBreakingDrafts) bounds Drafts() output even when a
// changelog carries more SEMVER-MAJOR entries than that — `-limit` at import time
// is the real gate, this is a defensive backstop.
func TestNodeBreaking_CapsEntriesPerRun(t *testing.T) {
	var b strings.Builder
	b.WriteString("## 2026-01-01, Version 99.0.0 (Current), @nobody\n\n### Commits\n\n")
	for i := 0; i < ingest.MaxNodeBreakingDrafts+50; i++ {
		fmt.Fprintf(&b, "* \\[[`%08x`](https://github.com/nodejs/node/commit/%08x)] - **(SEMVER-MAJOR)** **subsystem%03d**: change %d (Someone) [#%d](https://github.com/nodejs/node/pull/%d)\n", i, i, i, i, i, i)
	}
	src := ingest.NewNodeBreakingSource(
		ingest.WithNodeBreakingTargets([]string{"Vbig"}),
		stubNodeBreaking(map[string]string{"Vbig": b.String()}),
	)
	drafts, err := src.Drafts(context.Background())
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != ingest.MaxNodeBreakingDrafts {
		t.Fatalf("want %d drafts (capped), got %d", ingest.MaxNodeBreakingDrafts, len(drafts))
	}
}

// A draft must map to a schema-valid, quarantined, facts-only record through the
// ingest ladder, and a re-import against an index carrying it dedups (parity with
// eollive/npmlive).
func TestNodeBreaking_PrepareQuarantinesAndDedups(t *testing.T) {
	ctx := context.Background()
	src := ingest.NewNodeBreakingSource(
		ingest.WithNodeBreakingTargets([]string{"V22"}),
		stubNodeBreaking(map[string]string{"V22": nodeChangelogFixtureV22}),
	)
	drafts, err := src.Drafts(ctx)
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	if len(drafts) != 4 {
		t.Fatalf("want 4 drafts, got %d", len(drafts))
	}
	d := drafts[0]

	ix := openIx(t)
	meta := ingest.Meta{ID: "exp-0001", Author: "twiceshy-importer", Now: "2026-06-22", IncludeQuarantined: true}
	out, err := ingest.Prepare(ctx, ix, repo, d, meta)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if out.Novelty != index.NoveltyNovel {
		t.Fatalf("first Prepare want Novel, got %q", out.Novelty)
	}
	if out.Record == nil || out.Record.Status != "quarantined" {
		t.Fatalf("first Prepare must quarantine, got record=%v", out.Record)
	}
	if out.Record.Kind != "trap" {
		t.Errorf("kind = %q, want trap", out.Record.Kind)
	}
	if out.Record.Provenance.SourceLicense != record.SourceLicenseFactsOnly {
		t.Errorf("facts-only provenance not carried: %+v", out.Record.Provenance)
	}
	if err := record.Validate(out.Record); err != nil {
		t.Errorf("prepared record not schema-valid: %v", err)
	}

	// Re-import the same draft against an index carrying the quarantined record -> dedup.
	if err := ix.Rebuild(ctx, []*record.Record{out.Record}, repo); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	meta.ID = "exp-0002"
	out2, err := ingest.Prepare(ctx, ix, repo, d, meta)
	if err != nil {
		t.Fatalf("Prepare (second): %v", err)
	}
	if out2.Novelty == index.NoveltyNovel {
		t.Fatalf("second Prepare must dedup, got Novel")
	}
	if out2.Record != nil {
		t.Errorf("deduped Prepare must not create another record, got %+v", out2.Record)
	}
}
