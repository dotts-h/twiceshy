---
schema_version: 1
id: exp-0101
kind: fix
status: quarantined
title: setuptools deprecated pkg_resources in favor of importlib
symptom:
    summary: pkg_resources emits a DeprecationWarning and is slated for removal; use importlib.metadata/resources.
    error_signatures:
        - 'DeprecationWarning: pkg_resources is deprecated as an API'
applies_to:
    - ecosystem: PyPI
      package: setuptools
      versions:
        introduced: 67.5.0
        fixed: null
resolution:
    root_cause: pkg_resources is a slow, import-time-heavy runtime API that setuptools deprecated once the stdlib gained importlib.metadata (3.8+) and importlib.resources (3.9+).
    fix: Use importlib.metadata for version/entry-point lookups and importlib.resources for packaged data files instead of pkg_resources.
provenance:
    source:
        author: twiceshy-importer
        session: null
        pr: null
    recorded_at: "2026-06-18"
    validated_at: null
    valid:
        from: "2026-06-18"
        until: null
    source_license: none (facts only)
    source_url: https://setuptools.pypa.io/en/latest/pkg_resources.html
    superseded_by: null
---

setuptools deprecated pkg_resources: importing it warns and it is scheduled
for removal. Migrate version and entry-point queries to
importlib.metadata, and packaged-data access to importlib.resources (both in
the standard library on supported Pythons).
