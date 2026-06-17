# Security gate playbook — twiceshy

Security scanning is a **required CI gate**, not an advisory run —
`.forgejo/workflows/security.yml` runs on the same single-run trigger as
the quality gates. It catches the class of regression unit/contract/e2e tests
don't: a known-CVE dependency, an insecure code pattern, or a credential
committed to history. Make it a required check via the pr-gating recipe.

## What each gate does

- **SAST (static analysis)** — `go vet ./... ; govulncheck ./...`. Reads the source without
  running it and flags insecure patterns (injection sinks, unsafe deserialization,
  weak crypto, tainted data reaching a dangerous call). It complements lint: lint
  is about style/correctness, SAST is about *security* properties.
- **Dependency / vulnerability audit** — `govulncheck ./...`. Cross-checks
  your dependency tree (and, for tools like govulncheck, the *symbols you actually
  call*) against published advisories, so a vulnerable transitive package fails CI
  instead of shipping silently.
- **Secret scan (gitleaks)** — scans the diff (PRs) and history (pushes) for
  high-entropy strings and known credential shapes (API keys, tokens, private
  keys). A leaked secret is a credential to *rotate*, not just a line to delete —
  see triage below. Configured by `.gitleaks.toml`, whose allowlist exempts test
  fixtures.

## Run it locally (before you push)

```sh
# SAST + dependency audit — the exact commands CI runs:
go vet ./... ; govulncheck ./...
govulncheck ./...

# Secret scan over the working tree + history (install gitleaks first):
gitleaks detect --config .gitleaks.toml --redact
```

Running these locally turns a red CI into a pre-push fix and keeps the gate fast.

## Triage a finding

1. **Is it real?** Read the finding — the rule id, the file/line, the data flow.
   SAST and audit tools report *potential* issues; confirm the path is actually
   reachable and the input actually untrusted.
2. **If real, fix at the root.** Bump/patch the vulnerable dependency (or remove
   it); rewrite the insecure pattern; for a leaked secret, **rotate the credential
   first** (assume it is compromised the moment it hit a remote), then purge it
   from history (`git filter-repo`/BFG) and update `.gitleaks.toml` only if a
   *non-secret* fixture tripped the scanner.
3. **If it can't be fixed now,** open a tracked issue (issues recipe) with the
   severity and a deadline rather than suppressing it silently — a suppression
   with no owner is how a real vuln hides.

## Suppress a false positive (deliberately, with a paper trail)

A suppression is a security decision; it must be narrow, justified, and reviewable.

- **SAST** — use the tool's inline suppression scoped to the single line/rule
  (e.g. `// nosec G401` for gosec/`#nosec`, `# nosemgrep: <rule-id>` for semgrep,
  `# nosec` for bandit) **with a comment saying why**. Never disable a whole rule
  repo-wide to clear one finding.
- **Dependency audit** — record the accepted advisory in the tool's ignore file
  (`govulncheck` has no per-CVE ignore — pin/patch instead; `pip-audit
  --ignore-vuln`, `npm audit` resolutions, `trivy --ignorefile .trivyignore`)
  with an expiry/review date.
- **Secret scan** — add a path or regex to the `allowlist` in `.gitleaks.toml`
  for genuine test fixtures only. If it's a real key shape that happens to be
  fake, prefer an obviously-fake placeholder (`AKIAEXAMPLE…`) over an allowlist.

Every suppression is a review flag: the reviewer should see *why* in the diff.

## Backfill checklist

- [ ] Install the scanners CI needs in `security.yml` (govulncheck/semgrep/bandit/
      pip-audit/trivy/npm — match `go vet ./... ; govulncheck ./...` and `govulncheck ./...`).
- [ ] Set `go vet ./... ; govulncheck ./...` / `govulncheck ./...` to this repo's real tools.
- [ ] Make `security` a required status check (pr-gating recipe).
- [ ] Triage and fix (or track) the findings from the first full run.
- [ ] Rotate any credential the secret scan surfaces in existing history.
