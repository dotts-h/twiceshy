# Web sources for high-signal trap ingestion (GPT 5.5 research, 2026-06-27)

License-verified scoping of web sources twiceshy could ingest for engineering traps
(error→cause→fix), NOT security advisories. Ranked by license-safety × schema-fit × yield.

> **Known gap:** rows 1–7 of the original GPT 5.5 table were lost in transcription — the file
> was committed starting at row 8 (f58a41c's parent 33d81ee already lacked them). The four
> "Recommended adapters" below are presumed survivors of that block. Restore from a rerun if
> ever needed; the 2026-07-07 addendum below is independent.

| 8 | TypeScript TSxxxx diagnostics | **ingest-OK** — Apache-2.0; retain notices. [LICENSE](https://raw.githubusercontent.com/microsoft/TypeScript/main/LICENSE.txt) | **Mixed:** excellent signature index, but most entries lack explicit cause/fix. Expect enrichment or panel rejection. | ~2,000 raw; perhaps `10²–10³` usable | Parse [`diagnosticMessages.json`](https://raw.githubusercontent.com/microsoft/TypeScript/main/src/compiler/diagnosticMessages.json); map `code` and message template, then derive guards from compiler tests. | H/M |
| 9 | Go `vet` analyzers | **ingest-OK** — BSD-3-Clause. [x/tools license metadata](https://pkg.go.dev/golang.org/x/tools/go) | **Good but small:** analyzer name/docs and emitted diagnostics; suggested fixes exist for some analyzers. | ~30–50 (`10¹`) | Enumerate `go/analysis/passes`; `Analyzer` exposes name/docs and diagnostics may contain suggested fixes. [API](https://pkg.go.dev/golang.org/x/tools/go/analysis) | H |
| 10 | Django release notes | **ingest-OK** — BSD-3-Clause; retain notice. [LICENSE](https://raw.githubusercontent.com/django/django/main/LICENSE) | **Good:** dedicated backwards-incompatible and deprecation sections, often with replacement behavior. | `10²–10³` | Parse versioned reStructuredText/HTML headings from the [release-note index](https://docs.djangoproject.com/en/dev/releases/). Require old behavior + new behavior or migration text. | H/M |
| 11 | Python “What’s New” | **ingest-OK** — PSF License v2; retain copyright/license and summarize changes. Documentation code from Python 3.8.6 is also 0BSD. [CPython license](https://raw.githubusercontent.com/python/cpython/main/LICENSE) | **Good:** porting, removals, deprecations and changed behavior; error signatures are uncommon. | `10²–10³` | Parse versioned `Doc/whatsnew/*.rst`, restricting extraction to porting/deprecation/removal sections. [Index](https://docs.python.org/3/whatsnew/index.html) | H/M |
| 12 | Go release notes | **ingest-OK** — BSD-3-Clause. [LICENSE](https://raw.githubusercontent.com/golang/go/master/LICENSE) | **Good:** behavior and compatibility changes; usually lacks literal error text. | `10²` | Parse release-note HTML/source by version and retain affected package/API. [Release history](https://go.dev/doc/devel/release) | H/M |
| 13 | Next.js upgrade guides | **ingest-OK** — MIT. [LICENSE](https://raw.githubusercontent.com/vercel/next.js/canary/license.md) | **Good:** concrete before/after migrations, but prose structure varies by release. | `10²` | Parse versioned Markdown under the tagged repository’s docs; accept sections containing removed/renamed APIs plus replacement code. | H/M |
| 14 | Node.js changelogs/deprecations | **ingest-OK** for Node-authored material — MIT; avoid bundled third-party license sections. [LICENSE](https://github.com/nodejs/node/blob/main/LICENSE) | **Medium:** structured version and `SEMVER-MAJOR` markers, but many entries require linked-PR enrichment. | `10²–10³` | Parse tagged [`doc/changelogs`](https://github.com/nodejs/node/blob/main/CHANGELOG.md), deprecation codes and `SEMVER-MAJOR` entries; exclude security-only records. | H/M |
| 15 | React upgrade guides | **reframe-needed** — documentation is CC-BY-4.0, requiring attribution, license link and modification indication. [React docs license](https://github.com/reactjs/react.dev), [CC-BY-4.0 terms](https://creativecommons.org/licenses/by/4.0/legalcode.en) | **Medium:** strong migrations, but few stable error signatures and only major versions are archived. [Version policy](https://react.dev/versions) | `10¹–10²` | Parse upgrade/blog Markdown, distill facts under §5, and preserve page-level provenance and CC attribution. | H |
| 16 | GitHub closed issue → fix commit | **no-go for generic issue-prose ingestion**. Public-repository users receive only the license necessary to use/fork content through GitHub; do not assume the repository’s source-code license covers every issue comment. [GitHub Terms](https://docs.github.com/en/site-policy/github-terms/github-terms-of-service?lang=en) | Potentially excellent but noisy: issue symptom plus linked fixing PR/commit. | `10³–10⁴` from curated repositories | API access exists for issues, comments and PRs. [Issues API](https://docs.github.com/en/rest/issues), [Pull API](https://docs.github.com/en/rest/pulls). Mine licensed commits/tests; retain only independently derived facts and short functional signatures. Do not copy issue prose. | H structure / L legal boundary |
| 17 | Stack Overflow | **no-go for direct/raw ingestion; reframe-only pilot needs legal review.** Posts use CC-BY-SA 2.5, 3.0 or 4.0 according to revision date. [License matrix](https://stackoverflow.com/help/licensing) | Semantically excellent, but accepted answers are not reliably correct and often contain substantial copyrighted prose/code. | `10⁴–10⁵` candidates; much lower after validation | The API and CC data dump are available, but direct adaptation creates attribution/share-alike obligations. [Terms](https://stackoverflow.com/legal/terms-of-service), [API terms](https://stackoverflow.com/legal/api-terms-of-use). A feasible pilot would extract only non-copyrightable facts/short error strings, independently rewrite cause/fix, retain post/revision provenance, and quarantine everything. | H license / L reframe boundary |

## Recommended adapters

### 1. Rust error explanations

Fetch a pinned `rust-lang/rust` tag and parse `compiler/rustc_error_codes/src/error_codes/*.md`; validate membership against the [official error index](https://doc.rust-lang.org/error_codes/error-index.html).  
Map `E0xxx` plus emitted diagnostic text to `error_signatures`; explanation to `root_cause`; corrected example to `fix`; tag range becomes `applies_to.version-range`.  
Compile the bad and corrected examples where practical to create `guard.repro`.  
All imports are born `quarantined`; the panel promotes them later.

### 2. Ruff rules

Fetch a pinned Ruff tag and parse rule metadata/generated documentation; the [catalog](https://docs.astral.sh/ruff/rules/) exposes code, message and fix status.  
Map rule code/message to `error_signatures`, explanation to `root_cause`, suggested or automatic correction to `fix`, and Ruff/Python availability to `applies_to`.  
Use documented bad/good examples as `guard.repro`; discard style-only and security-only rules.  
All imports are born `quarantined`; the panel promotes them later.

### 3. Clippy lints

Fetch a pinned `rust-clippy` release and parse lint declarations plus generated [lint documentation](https://doc.rust-lang.org/clippy/lints.html).  
Map lint name and warning template to `error_signatures`, “why” text to `root_cause`, corrected example/suggestion to `fix`, and toolchain tag to `applies_to.version-range`.  
Compile examples with `cargo clippy` to produce guards and verify the warning disappears after correction.  
All imports are born `quarantined`; the panel promotes them later.

### 4. Node.js SEMVER-MAJOR changelog

Fetch `nodejs/node`'s per-major-line changelog files (`doc/changelogs/CHANGELOG_V<N>.md`) for the current + active-LTS majors — MIT, Node-authored changelog text only (row 14; the GitHub-issues no-go does not apply, this is repo-authored documentation, not issue prose).  
Parse each release section (`## <date>, Version <version>...`) for commit lines carrying the literal `(SEMVER-MAJOR)` marker; map subsystem + subject to `error_signatures`/`title`, the linked PR to `fix`, and the changelog file (+ version anchor) to `source_url`.  
Only the factual subsystem/subject naming the change is used; no changelog prose beyond that short factual line is reproduced.  
All imports are born `quarantined`; the panel promotes them later — implemented as `node-breaking` (#0115).

## Explicit no-gos

- Do not build a generic GitHub issue/comment text importer. API accessibility is not a redistribution license.
- Do not import Stack Overflow questions, answers or code directly into the AGPL corpus. CC-BY-SA attribution and share-alike obligations are record-level and revision-dependent.
- Do not treat CC-BY material such as React documentation as public domain. It requires §5 reframing plus provenance and attribution.

## Addendum 2026-07-07 — coverage-gap sources (Gemini research, every URL re-verified locally)

Targeted at the #0088 zero-coverage areas. Artifact + license URLs curl-verified 2026-07-07
(two of the research pass's URLs were wrong and are corrected below — verify-cited-URLs stands).

| A1 | Swift compiler diagnostics | **ingest-OK** — Apache-2.0. [LICENSE](https://raw.githubusercontent.com/apple/swift/main/LICENSE.txt) | **High:** structured `ERROR(id, opts, "signature")` macros; fix hints in adjacent `NOTE` macros. First iOS/native source. | `10³` | Regex-extract `ERROR`/`WARNING`/`NOTE` macros from [`DiagnosticsSema.def`](https://raw.githubusercontent.com/apple/swift/main/include/swift/AST/DiagnosticsSema.def) + sibling `.def` files at a pinned tag. | H |
| A2 | KubePug Kubernetes deprecations | **ingest-OK** — Apache-2.0. [LICENSE](https://raw.githubusercontent.com/kubepug/kubepug/main/LICENSE) | **Excellent:** group/kind, description, deprecated/removed version, explicit `replacement`. Cleanly bypasses the CC-BY k8s website. | `10²` | Parse the generated [`data.json`](https://kubepug.xyz/data/data.json) (NOTE: lives on kubepug.xyz per their README, not in the repo; server requires a browser User-Agent). | H |
| A3 | ESLint core rules | **ingest-OK** — MIT. [LICENSE](https://raw.githubusercontent.com/eslint/eslint/main/LICENSE) | **High:** each rule exports `meta.messages` (error signatures) + `meta.docs` (cause); fixable flag marks auto-fixes. | ~300 (`10²`) | AST/regex-parse `lib/rules/*.js` at a pinned tag for the `meta` object literals. | H |
| A4 | uv resolver/CLI errors | **ingest-OK** — MIT/Apache-2.0 dual. [LICENSE-MIT](https://raw.githubusercontent.com/astral-sh/uv/main/LICENSE-MIT) | **High:** `#[error("…")]` gives the signature, `#[diagnostic(help("…"))]` the fix, in the same attribute block. | `10²` | Scan `crates/**/*.rs` at a pinned tag for `thiserror`/`miette` attribute pairs. | H |
| A5 | deno_lint rules | **ingest-OK** — MIT. [LICENSE](https://raw.githubusercontent.com/denoland/deno_lint/main/LICENSE) | **Good:** 122 rule files with code/message/hint; mine the source, NOT the CC-BY docs.deno.com rendering. | `10²` | Parse `src/rules/*.rs` (the research pass cited a nonexistent `mod.rs` — the directory is the artifact). | H |
| A6 | React production error codes | **ingest-OK (signatures only)** — MIT. [LICENSE](https://raw.githubusercontent.com/facebook/react/main/LICENSE) | **Medium:** [`codes.json`](https://raw.githubusercontent.com/facebook/react/main/scripts/error-codes/codes.json) maps error IDs to message templates; the fix prose lives in CC-BY react.dev docs, so fixes must be independently synthesized (row 15's reframe rule). | `10²` | Parse the JSON map; enrich cause/fix from first principles + executed repros, never doc prose. | H/M |

**Experience-shaped sources (the differentiated material — filed as issues, higher priority
than the catalogs above):**

- **External-OSS git fail→fix mining (#0133)** — row 16's carve-out ("mine licensed
  commits/tests") applied at scale: fix-shaped commits in permissively-licensed high-star
  repos ARE experience records (author's own trap account + validated fix + guard test).
  License allowlist gate; never the issue/PR prose. Yield `10³–10⁴`.
- **wtfjs / wtfpython (#0134)** — both SPDX `WTFPL` (verified 2026-07-07): curated
  symptom→cause gotcha collections, ingest-OK as-is. Yield `10²` each.

**Additional no-gos confirmed 2026-07-07:** `teivah/100-go-mistakes` (NOASSERTION — book
content, no license); MIT link-lists like `charlax/professional-programming` (the list is MIT,
the linked blog content is not); Deno's rendered docs site (mine the MIT source instead).
Android/Kotlin, React Native/Expo-specific, and Docker/Postgres/Terraform sources returned
nothing this pass (Codex quota-limited) — rerun there.
