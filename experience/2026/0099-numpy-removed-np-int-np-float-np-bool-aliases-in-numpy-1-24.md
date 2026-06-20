---
schema_version: 1
id: exp-0099
kind: fix
status: quarantined
title: numpy removed the np.int / np.float / np.bool aliases in NumPy 1.24
symptom:
    summary: Code using np.int (and the np.float/np.bool aliases) raises AttributeError on NumPy 1.24+.
    error_signatures:
        - 'AttributeError: module ''numpy'' has no attribute ''int'''
applies_to:
    - ecosystem: PyPI
      package: numpy
      versions:
        introduced: 1.24.0
        fixed: null
resolution:
    root_cause: The np.int/np.float/np.bool aliases for the Python builtins were deprecated in NumPy 1.20 and removed in 1.24; they only ever aliased the builtins and added no value.
    fix: Use the Python builtins int/float/bool, or an explicit sized dtype such as np.int64/np.float64 where width matters.
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
    source_url: https://numpy.org/devdocs/release/1.24.0-notes.html#expired-deprecations
    superseded_by: null
---

np.int, np.float and np.bool were deprecated aliases removed in NumPy 1.24,
so any code referencing them raises AttributeError. Replace them with the
plain builtins (int, float, bool) or an explicit NumPy dtype (np.int64,
np.float64) when a specific width is required.
