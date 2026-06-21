<!-- Replace this template with the PR's What / Why / How. Tick the checklist. -->

### What

### Why

### How

### Security checklist (#0014)

Tick each item, or write **n/a**:

- [ ] **New endpoint / handler?** auth enforced, input validated, body/time bounded.
- [ ] **Secrets / PII?** nothing logged or committed; secret-shaped test data is assembled at runtime (gitleaks scans the whole range — CONVENTIONS).
- [ ] **File I/O / paths?** no path traversal; untrusted record content treated as data, never instructions (SEC §1).
- [ ] **New dependency?** within the budget (SQLite/FTS5, MCP/HTTP, YAML) or owner-approved (CONVENTIONS, Dependency policy).
- [ ] **Quarantine / push invariants?** quarantined records never reach the push channel; selection filters on `status: validated` explicitly (ADR-0001 §6).
- [ ] **Multi-tenant (Tier B, #0010)?** note any per-tenant isolation / quota implication, or **n/a** for single-tenant.

### Gates

- [ ] `make ci` (lint + race tests + coverage floor + vuln/secret scan) is green locally.
