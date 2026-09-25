---
id: CA-935b
title: Audit/doc integrity — ledger blocks, LSP test-file revert, done-doc metadata, baseline triage (BUG-442, BUG-443, BUG-444, BUG-445 triage)
type: BugFix
feature: dev-infra
date: 2026-09-23
status: done
---

## Context

- BUG-442: nine wave CAs (916–920, 925–928) were missing the required
  `flowpilot:change-ledger` block, and four feature keys used across the wave
  (`provider-runtime`, `flow-gates`, `engine-init`, `test-suite-health`) were
  not registered in `change-audit/FEATURE-KEYS.md`.
- BUG-443: CA-918b edited the pre-existing `lsp/server_manager_test.go`
  (inserted notification recording into `lspHelperServe` + added
  `lspHelperRecordMethod`) contrary to the additive-tests-only rule.
- BUG-444: fourteen `done/` reports still read `Status: open` with stale
  `Last Updated`/`Current Ask: awaiting prioritization`, and two files shared
  BUG-400 (canonical report + an orphaned 5-line completion note).
- BUG-445: the wave was declared done while baseline suites remained red —
  zero regression delta is not the same as the contract's green-old-tests gate.

## Change

- `change-audit/FEATURE-KEYS.md`: registered `flow-gates` (flow-gate
  enforcement layer: tier-1 doc-scope rules, tier-2/3 audit gates,
  oracle/reproduce/replay gates).
- CA-916b/917 `feature:` migrated `provider-runtime` → `ai-providers`
  (registered key, same adapter scope); CA-925b `engine-init` →
  `skill-anchored-init`; CA-927b `test-suite-health` → `change-contract`
  (canonical-head validation is explicitly that key's scope). Ledger blocks
  appended to all nine missing CAs with accurate `source_doc_id`s.
- `lsp/server_manager_test.go` restored byte-identical to HEAD
  (`git diff --exit-code` clean). The BUG-380 handshake instrumentation moved
  into the additive fixture `bug380_initialized_handshake_test.go` via its own
  helper entry point (`TestBUG380HelperProcess` + `bug380HelperServe` +
  `bug380RecordMethod`) — same assertions, no pre-existing file touched.
- 14 done reports reconciled (`Status: done`, `Last Updated: 2026-09-23`,
  Current Ask → completion notes). BUG-400 completion evidence merged into the
  canonical report; the duplicate file's removal awaits operator confirmation
  (destructive-op rule).
- BUG-445 triage executed: full `./internal/...` run on tree + identical
  `-run` set on baseline `d191004f`. Result: 19 baseline-identical
  deterministic defects captured in BUG-454, 6 env-dependent reds proposed as
  expiry-bound waivers (2026-10-23), 6 TempDir-cleanup flakes documented.
  BUG-445 stays `open` pending operator waiver sign-off; the suite-green gate
  is honestly reported as unsatisfied.

## Tests

- `go test -count=1 ./internal/lsp` — all green (BUG-380 handshake test passes
  through the new own-helper path; `server_manager_test.go` untouched).
- Ledger-block scan: 13/13 wave CAs (916–928) + 929–934 carry exactly one
  block; every `feature_key` resolves in FEATURE-KEYS.md (verified via
  comm-diff: zero unregistered).

## Result

- Audit/doc integrity restored without weakening any test: the reverted file
  is byte-identical to baseline and all its assertions still run.
- The wave's completion claim is now honest: every fix has a ledgered CA, done
  docs are reconciled, and the remaining red suite is captured as triaged debt
  (BUG-454 + waiver table) instead of a silent "zero delta".

# ---8<--- flowpilot:change-ledger
feature_key: dev-infra
source_doc_id: BUG-442
change_type: bugfix
summary: Ledger blocks + feature-key registration for wave CAs; LSP test-file revert to byte-identical baseline; done-doc metadata reconciled; baseline red suite triaged into BUG-454 + waivers
# --->8---
