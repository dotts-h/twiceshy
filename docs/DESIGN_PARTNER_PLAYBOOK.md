# Design-partner playbook

This is the operator package for recruiting and running the first five twiceshy
design-partner conversations. It makes no claim that twiceshy already improves team
outcomes. The purpose of each pilot is to test that claim using the
[design-partner measurement protocol](PILOT_PROTOCOL.md).

No outreach is automated by this repository. A human reviews and sends every message.

## Qualification

A strong candidate meets all required criteria and at least three preferred criteria.

Required:

- An engineering team already uses coding agents on real work at least weekly.
- A person authorized by the organization agrees to the pilot and data-handling plan.
- The team can run a non-overlapping baseline and treatment with comparable agents,
  repositories, and workflows.
- The team can provide salted session/query hashes and structured judgements without
  sending raw prompts, transcripts, source code, secrets, personal identifiers, or
  repository URLs.
- One engineering owner will review outcome judgements and operational confounders.

Preferred:

- The team recognizes repeated agent failures, CI retries, or rediscovery of prior fixes.
- It has enough error-triggered agent activity for both windows to produce useful samples.
- It can keep the same telemetry salt and observer configuration across both windows.
- It is willing to discuss private team knowledge, self-hosting, or VPC requirements.
- It can complete a results review within one week after treatment ends.

Do not enroll a team when authorization is unclear, raw sensitive data is required, the
workflows will materially change between arms, or a safety/incident response owner is
unavailable. Log the reason as `not-qualified`; do not retain unnecessary details.

## Five-target tracker

Use [the CSV tracker](templates/design-partner-tracker.csv) as the working copy. It has
five empty target slots and contains no invented organizations or contacts.

Allowed pipeline states are `research`, `qualified`, `contact-approved`, `contacted`,
`replied`, `pilot-agreed`, `baseline`, `treatment`, `review`, `complete`, `declined`, and
`not-qualified`. Record dates in `YYYY-MM-DD`. Keep personal contact details in the
organization's approved CRM or address book, not in this repository.

At each weekly review, record only the next action, its owner, and due date. A target
does not move to `contact-approved` until the qualification and consent pre-checks pass.

## Consent and data-handling checklist

Complete this before baseline collection. This is an operational checklist, not legal
advice; the operator and partner should involve qualified counsel where required.

- [ ] Name the organizations responsible for data and their security/privacy contacts.
- [ ] Document the purpose: evaluate engineering-agent failure prevention, not employee
      performance, surveillance, model training, advertising, or billing.
- [ ] Identify the lawful/contractual basis and obtain organizational authorization and
      participant notice or consent as applicable.
- [ ] Agree exactly which teams, repositories, agents, and dates are in scope.
- [ ] Confirm collection is limited to salted hashes, record IDs, timestamps, decisions,
      and structured `used`/`confirmed`/`incorrect` judgements.
- [ ] Confirm raw prompts, transcripts, source code, stack traces, evidence text, tokens,
      names, emails, raw session IDs, and repository URLs are excluded.
- [ ] Treat salted hashes as pseudonymous, potentially personal data—not anonymous data.
- [ ] Agree salt custody, rotation after the pilot, storage location, encryption, access
      list, retention period, deletion date, and backup-deletion handling.
- [ ] Define participant opt-out, correction, access, and deletion handling.
- [ ] Define the incident-reporting route and the action for suspected secret/raw-data
      capture: stop collection, isolate the artifact, notify the owners, and investigate.
- [ ] Agree whether results remain private. Obtain separate written approval before using
      a partner name, logo, quotation, case study, or identifiable metric publicly.
- [ ] Record sign-off from the twiceshy operator and the partner owner before collection.

## Onboarding checklist

### Before baseline

- [ ] Confirm qualification and consent/data-handling sign-off.
- [ ] Choose comparable, non-overlapping windows and pre-register them in UTC.
- [ ] Choose opaque team labels and create the hashed-session cohort map.
- [ ] Install the baseline observer: it emits privacy-safe error decisions with no served
      cards and never injects context into the agent.
- [ ] Verify raw-query capture is off and inspect a sample JSONL line for prohibited data.
- [ ] Test telemetry rotation, clock synchronization, access controls, and deletion.
- [ ] Record known confounders and freeze avoidable workflow/configuration changes.
- [ ] Run a dry report and confirm unattributed sessions and malformed outcomes are visible.

### Baseline-to-treatment handoff

- [ ] Close the baseline at the pre-registered exclusive end timestamp.
- [ ] Archive a read-only baseline input snapshot and note telemetry drops/outages.
- [ ] Enable twiceshy injection without changing the observer, salt, team labels, agents,
      or repositories unless a change is necessary and documented.
- [ ] Train reviewers on `used`, `confirmed`, `incorrect`, and unjudgeable outcomes using
      the canonical definitions in the pilot protocol.
- [ ] Confirm the stop route for incorrect/harmful advice and privacy incidents.

### During treatment and close-out

- [ ] Review safety reports promptly; do not wait for the final analysis.
- [ ] Monitor outcome coverage and unattributed sessions without reading raw content.
- [ ] Close at the pre-registered timestamp and run deterministic JSON and CSV reports.
- [ ] Verify the JSON/CSV inputs, command, tool commit, exclusions, and confounders are
      recorded so the result is reproducible.
- [ ] Hold a partner review, agree follow-up actions, then delete or retain data according
      to the signed handling plan.

## Outreach drafts

Replace bracketed fields with verified facts. Delete any sentence that cannot be
supported. Do not imply a customer relationship, measured benefit, security guarantee,
or production readiness that does not exist.

### Direct email

Subject: Measure whether coding agents repeat fewer engineering failures

> Hi [name] — I am building twiceshy, an experience service that gives coding agents
> short, validated engineering lessons when a matching failure appears. We have built
> the retrieval, validation, privacy-safe telemetry, and baseline/treatment reporting,
> but we have not yet established team-level impact. I am looking for a small number of
> design partners willing to run a measured pilot with raw prompts and source code kept
> out of the reporting data. Would a 25-minute fit and data-handling discussion be useful?
> If it is not relevant, no reply is needed.

### Show HN draft

Title: Show HN: twiceshy – test whether coding agents stop repeating known failures

> twiceshy is an AGPL Go service that serves short engineering trap/fix records to coding
> agents over MCP and a high-precision push hook. It has a Git-backed corpus, quarantine
> and validation gates, reproducible guards, and privacy-safe decision telemetry. The
> open question is outcome impact: we do not yet have enough external pilot evidence to
> claim that teams resolve failures faster or repeat fewer errors. We have added a public
> baseline/treatment protocol and deterministic confidence-aware reports, and are looking
> for design partners who already use coding agents on real engineering work. Raw prompts,
> transcripts, source code, and secrets are excluded from the measurement inputs. Repo:
> [verified repository URL]. Pilot details: [verified protocol URL].

### Community post draft

> Looking for up to five engineering teams that already use coding agents and see repeated
> failures or rediscovery of old fixes. twiceshy serves validated lessons at decision time;
> we are testing—not assuming—that this lowers a privacy-safe repeated-error proxy without
> increasing incorrect advice. The pilot uses matched baseline/treatment windows, salted
> hashes, structured outcomes, and 95% confidence intervals; no raw prompts or source code
> are required. If your team can appoint an engineering/data-handling owner, details are at
> [verified protocol URL], or reply privately for a fit check.

## Results-review decision rubric

Pre-register the partner's safety ceiling and minimum usable sample before viewing the
result. The report provides Wilson intervals, not a causal model; operational changes and
missing judgements remain confounders.

Apply the rubric in order:

1. **Stop / do not expand** when a privacy incident occurred, a credible severe harmful
   recommendation remains unresolved, authorization was invalid, or treatment breaches
   the pre-agreed incorrect-advice safety ceiling. Preserve only incident evidence the
   handling plan authorizes.
2. **Inconclusive / fix measurement** when windows or cohorts are not comparable,
   telemetry loss is material, outcome coverage misses the pre-registered minimum,
   unattributed traffic dominates, or confidence intervals are too wide for the decision.
   Do not present a point estimate as evidence of improvement.
3. **Iterate product** when data quality is adequate and the repeated-error proxy moves in
   the desired direction, but intervals overlap materially, gains are isolated to one team
   or record, hit rate is low, or incorrect-rate uncertainty is still commercially unsafe.
4. **Proceed to a larger/paid validation** only when multiple independently qualified
   teams show a consistent reduction in repeated-error proxy, confidence bounds are useful
   for the pre-registered decision, incorrect advice stays within the safety ceiling, and
   partner reviews agree the cards changed useful engineering actions. This authorizes a
   commercial experiment, not a public efficacy claim.
5. **Publish a claim** only after a separate review verifies the analysis, limitations,
   consent for disclosure, and exact wording. Report sample sizes, windows, outcome
   coverage, confidence intervals, and confounders alongside the claim.

The review record should end with one decision—`stop`, `measurement-retry`,
`product-iterate`, `larger-validation`, or `publish-review`—plus an owner, due date, and
the evidence that supports it.
