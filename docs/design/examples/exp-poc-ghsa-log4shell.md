<!--
  PROOF-OF-CONCEPT — not blessed corpus. Uses the PROPOSED schema
  (provenance.source_license / source_url, see docs/design/corpus-bootstrap.md
  §4). Not under experience/, so not indexed/validated.

  Source tier: GitHub Advisory Database (CC-BY-4.0) / OSV. This is the
  APPLIES_TO-BACKBONE, QUARANTINED-PULL-ONLY shape: OSV's affected-version
  ranges map almost 1:1 onto twiceshy's applies_to, but there is no
  auto-synthesizable fail-to-pass guard (running the actual exploit is not a
  unit test), so it stays quarantined and pull-only. CC-BY requires
  attribution, recorded in source_license/source_url.
-->
---
schema_version: 1
id: exp-poc-0002
kind: trap
status: quarantined
title: "Log4Shell: log4j-core 2.x JNDI lookup in logged strings allows RCE (CVE-2021-44228); upgrade to 2.17.1+"

symptom:
  summary: >
    Apache Log4j 2 evaluates JNDI lookup expressions embedded in logged
    strings. Any attacker-controlled value that reaches a log call (a header,
    a username, a User-Agent) containing `${jndi:ldap://...}` triggers a remote
    class load and arbitrary code execution. There is no obvious error — the
    sink is an ordinary `log.info(userInput)`.
  error_signatures:
    - "${jndi:ldap://"
    - "${jndi:rmi://"

applies_to:
  - ecosystem: "Maven"
    package: "org.apache.logging.log4j:log4j-core"
    versions: { introduced: "2.0-beta9", fixed: "2.17.1" }

resolution:
  root_cause: >
    Message lookup substitution was enabled by default, so the layout
    interpolated `${...}` expressions found in the *message* (not just in
    config), and the JNDI lookup resolver would fetch and deserialize a remote
    object. The primary fix shipped in 2.15.0; 2.16.0 disabled message lookups
    and removed the JNDI resolver; follow-up CVEs pushed the safe floor to
    2.17.1 (and 2.12.4 / 2.3.2 on older branches).
  fix: >
    Upgrade log4j-core to >= 2.17.1 (or the patched 2.12.4 / 2.3.2 on Java
    7/older branches). Mitigations for unpatchable systems (removing the
    JndiLookup class, setting log4j2.formatMsgNoLookups) are stopgaps, not
    substitutes for the upgrade.
  dead_ends:
    - tried: "setting log4j2.formatMsgNoLookups=true / LOG4J_FORMAT_MSG_NO_LOOKUPS as the fix"
      why_it_failed: >
        Incomplete: it blocks the message-lookup vector but later CVEs
        (CVE-2021-45046, CVE-2021-45105) showed other reachable paths; only the
        version upgrade closes the family.
    - tried: "upgrading to 2.15.0 and stopping"
      why_it_failed: >
        2.15.0 still had a reachable JNDI path (CVE-2021-45046) and a DoS
        (CVE-2021-45105); the safe floor is 2.17.1.

guard:
  repro: null
  guarding_test: null

provenance:
  source: { author: "twiceshy-importer", session: null, pr: null }
  source_license: "CC-BY-4.0"
  source_url: "https://github.com/advisories/GHSA-jfh8-c2jp-5v3q"
  recorded_at: 2026-06-13
  validated_at: null
  valid: { from: 2021-12-10, until: null }
  superseded_by: null
  usage: { retrieved: 0, confirmed_helpful: 0, last_hit: null }
---

## The trap

A coding agent adds logging of a request field — `log.info("login: " + user)`
— with no idea that on a vulnerable log4j-core the string `${jndi:ldap://...}`
in `user` is not data but an instruction. The vulnerable version range is wide
(`2.0-beta9` through the 2.16 line), and the symptom at write time is *nothing*
— the code looks correct and tests pass.

## Why it's quarantined and pull-only

This record is the **OSV/GHSA backbone** shape. Its `applies_to` comes
verbatim from the advisory's affected ranges (`introduced: 2.0-beta9`,
`fixed: 2.17.1`), and its source is the GitHub Advisory Database under
**CC-BY-4.0**, recorded in `source_license`/`source_url` for attribution and
for the pack builder's commercial-pack filter (CC-BY is permissive → allowed).

But there is **no auto-synthesizable fail-to-pass guard**: a working exploit
needs a malicious LDAP/RMI endpoint and is not a unit test, so per twiceshy's
lifecycle this record cannot be promoted to `validated` and therefore stays
`quarantined` — surfaced only on explicit pull queries, never on the push
channel. The `${jndi:` markers are recorded as `error_signatures` so an agent
that *pastes such a payload into a test fixture* could still get a pull-side
hit, but the value here is the version-range fact, delivered on demand.

## What it demonstrates for the importer

The version-range fact, the dead-ends (the "set formatMsgNoLookups" and "stop
at 2.15.0" partial fixes the literature is full of), and the bi-temporal
`valid.from` (advisory date) are all machine-derivable from one OSV/GHSA
record. Multiply across the advisory database and you have a large, clean,
pull-channel safety net — quarantined by design until a guard exists.
