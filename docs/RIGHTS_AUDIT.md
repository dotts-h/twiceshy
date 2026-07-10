# Rights audit and remediation workflow

`twiceshy rights-audit` is the read-only precondition for deciding whether
records can enter a commercial experience pack. It uses the same
`pack.ClassifyRecord` policy as `twiceshy pack`; it does not infer ownership,
apply a license, or modify the corpus.

## Audit and queue review

Run the audit against a corpus checkout and retain both artifacts:

```sh
twiceshy rights-audit \
  -corpus /path/to/twiceshy-corpus \
  -json \
  -queue /tmp/twiceshy-rights-remediation.json \
  > /tmp/twiceshy-rights-audit.json
```

The report contains stable reason buckets plus one finding per record. The
remediation queue contains only records whose evidence is missing, incomplete,
or unrecognized. Every queue item has `automatic_change:false`: a reviewer must
investigate provenance and submit any truthful metadata correction through a
normal corpus PR. The command never proposes `none (project-authored)` and never
rewrites a record.

Use the CI posture after the baseline is understood:

```sh
twiceshy rights-audit \
  -corpus /path/to/twiceshy-corpus \
  -json \
  -queue /tmp/twiceshy-rights-remediation.json \
  -fail-on-unknown
```

The report and queue are written before the non-zero exit, so CI can retain them
as artifacts. Known exclusions such as copyleft or internal-only records remain
visible reason buckets but do not count as unknown evidence.

## Validate a commercial pack

Build the pack, then verify that its selected records and complete notice ledger
still match the audited corpus:

```sh
rm -rf /tmp/twiceshy-commercial-pack
twiceshy pack \
  -corpus /path/to/twiceshy-corpus \
  -out /tmp/twiceshy-commercial-pack \
  -commercial

twiceshy rights-audit \
  -corpus /path/to/twiceshy-corpus \
  -manifest /tmp/twiceshy-commercial-pack/MANIFEST.json \
  -notices /tmp/twiceshy-commercial-pack/ATTRIBUTION.md \
  -json
```

Manifest selection or notice drift fails the command. Both files are required
together.

## Live-corpus smoke check

The engine repository does not own the live corpus. Point the smoke run directly
at its separate checkout; do not create an `experience/` symlink:

```sh
make build
LIVE_CORPUS=${TWICESHY_LIVE_CORPUS:-../twiceshy-corpus}
./twiceshy rights-audit \
  -corpus "$LIVE_CORPUS" \
  -json \
  -queue /tmp/twiceshy-live-rights-remediation.json \
  > /tmp/twiceshy-live-rights-audit.json

jq '{total_records, commercial_eligible, unresolved_evidence, reason_buckets}' \
  /tmp/twiceshy-live-rights-audit.json
git -C "$LIVE_CORPUS" status --short
```

The final command should show the same pre-existing worktree state as before the
audit. A current unresolved count is a remediation backlog, not permission to
fill metadata automatically.
