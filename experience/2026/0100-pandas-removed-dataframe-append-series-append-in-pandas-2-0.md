---
schema_version: 1
id: exp-0100
kind: fix
status: quarantined
title: pandas removed DataFrame.append / Series.append in pandas 2.0
symptom:
    summary: DataFrame.append and Series.append were removed in pandas 2.0; calls raise AttributeError.
    error_signatures:
        - 'AttributeError: ''DataFrame'' object has no attribute ''append'''
applies_to:
    - ecosystem: PyPI
      package: pandas
      versions:
        introduced: 2.0.0
        fixed: null
resolution:
    root_cause: append was deprecated in pandas 1.4 and removed in 2.0 because it reallocated on every call; concat is the supported, more efficient API.
    fix: Build a list of frames and call pd.concat(frames) once instead of repeated df.append calls.
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
    source_url: https://pandas.pydata.org/docs/whatsnew/v2.0.0.html#removal-of-prior-version-deprecations-changes
    superseded_by: null
---

pandas 2.0 removed DataFrame.append and Series.append (deprecated since
1.4), so calls now raise AttributeError. Collect the pieces and call
pd.concat once — repeated append was quadratic anyway.
