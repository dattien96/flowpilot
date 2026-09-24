---
id: BUG-442
title: CA-916b to CA-928b omit required ledger blocks and registered feature keys
status: done
version: 2
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [CP-Test-Progress-Tracking]
---

## AI Quick View
- **What**: Nine of 13 change audits lack parseable ledger metadata; four feature keys are unregistered.
- **Why**: Header `feature:` was used in place of the required `flowpilot:change-ledger` block.
- **Key constraint**: Preserve history and register/migrate feature identities without silently renaming them.

## 1. Metadata
- Document ID: `BUG-442`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `change-contract`, `context-regression-engine`
- Parent Documents: [CP-Test-Progress-Tracking](../../07-Coding-Plan/done/CP-Test-Progress-Tracking.md)

## 2. Symptom and Impact
`internal/skillpack/flow-pack/common/audit-logging/SKILL.md:15-45` requires a registered feature key and `# ---8<--- flowpilot:change-ledger` block with `source_doc_id`. Among CA-916b…CA-928b only CA-921b…CA-924b contain this marker; CA-916b…CA-920b and CA-925b…CA-928b omit it. `provider-runtime` (CA-916b/917), `flow-gates` (CA-919b/920), `engine-init` (CA-925b), `test-suite-health` (CA-927b) are not in FEATURE-KEYS.md. `changeledger.EnrichAll` (`enrich.go:21-28,62-73`) falls back to low-confidence heuristics without a matching CA block. Severity: **medium** for feature-history/audit traceability.

## 3. Reproduction and Evidence
Search for `flowpilot:change-ledger` in the 13 CA files → only CA-921b, 922, 923, 924 match. Compare each `feature:` header with FEATURE-KEYS.md; four key values are missing. Frontmatter is not the ledger parser's marker.

## 4. Acceptance and Verification
Audit all CA-916b…928, add valid ledger block and source_doc_id for each logical change, register or consciously migrate feature keys; test parser/enrichment against the actual notes. Not fixed here.

## 5. Resolution (2026-09-23, CA-935b)

- Ledger blocks appended to CA-916b…920, 925–928 (9 files); `flow-gates`
  registered in FEATURE-KEYS.md; feature migrations: provider-runtime→
  ai-providers (916/917), engine-init→skill-anchored-init (925),
  test-suite-health→change-contract (927).
- Verified: every wave CA (916–935) carries exactly one ledger block; zero
  unregistered `feature_key` values (comm-diff against FEATURE-KEYS.md).
