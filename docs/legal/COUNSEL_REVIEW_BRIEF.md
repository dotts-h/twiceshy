# Counsel review brief — corpus and commercial packs

> **DRAFT — NOT LEGAL ADVICE. No lawyer or counsel has approved this document,
> the contribution terms, the pack policy, or any commercial distribution.**
> This is an operator checklist for a qualified lawyer to review and replace or
> approve in writing before commercial launch.

## Decisions requested from counsel

### 1. Corpus contribution terms

- Identify the contracting legal entity and governing law.
- Review [CONTRIBUTION_TERMS.md](CONTRIBUTION_TERMS.md), including the inbound
  license grant, relicensing authority, employer-authorized contributions,
  patent language, moral-rights treatment, and agent-submitted records.
- Decide whether existing records need retroactive contributor acceptance or a
  separate provenance/ownership review.
- Specify evidence and retention requirements for acceptance, terms version,
  timestamp, contributor identity, and employer authority.

### 2. Commercial pack terms

- Draft the pack-level `LICENSE` delivered with every commercial pack.
- Define customer rights, restrictions, warranty/liability language,
  termination, updates, and treatment of third-party material.
- Confirm that pack terms do not purport to remove rights recipients retain
  under third-party licenses.

### 3. Third-party notice policy

- Approve the allowlist of source licenses and the exact obligations for each.
- Review whether the mechanical requirements in [RIGHTS_AUDIT.md](../RIGHTS_AUDIT.md)
  are sufficient for MIT, Apache-2.0, CC-BY, and any other admitted license.
- Define how canonical license text, copyright notices, Apache NOTICE material,
  CC-BY creator/title/license-link/change details, and source URLs must be
  captured and bundled.
- Decide whether a statement that upstream supplied no NOTICE file is acceptable
  evidence and how that statement must be documented.

### 4. Provenance evidence

- Define acceptable proof for `none (facts only)` and
  `none (project-authored)`; the software must never infer either status.
- Set requirements for immutable source revisions, rights-holder identity,
  copied/adapted expression, contributor grants, and review sign-off.
- Define retention periods and who may approve a remediation queue item.
- Review the `twiceshy-rights-v1` human-attestation fields and decide whether
  reviewer qualifications, dual control, source snapshots, or stronger signing
  are required. The attestation is not currently a cryptographic signature and
  does not claim counsel review. Its evidence digest mechanically covers the
  canonical full distributed record and path (excluding only the digest itself),
  but that integrity check does not prove reviewer identity or legal authority.

### 5. Takedown and dispute process

- Design intake channels for copyright, attribution, privacy, and ownership
  complaints, including required claimant information.
- Set response deadlines, temporary quarantine rules, escalation ownership,
  counter-notice/dispute handling, audit logging, and customer notification.
- Decide how already-distributed packs and later corrected/superseded records are
  handled contractually and operationally.

## Unresolved launch blockers

- Legal entity and governing terms remain undecided.
- Contribution terms are still draft and existing-record coverage is unknown.
- Final commercial pack license and customer agreement do not yet exist.
- The admitted third-party license allowlist and notice templates lack counsel
  approval.
- Facts-only/project-authored evidence standards and reviewer authority are not
  finalized.
- Takedown, dispute, retention, and previously distributed pack procedures are
  not adopted.

The rights-audit tooling is a fail-closed evidence and packaging control. It is
not a substitute for these legal decisions or for counsel review.
