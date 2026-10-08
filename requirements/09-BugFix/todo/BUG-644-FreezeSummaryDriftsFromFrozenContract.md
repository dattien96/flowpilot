# BUG-644 — Freeze note claims paths are declared in the frozen contract while the contract JSON omits them, so the planner's written record and the machine contract disagree

- **ID:** BUG-644
- **Severity:** Medium — every leg downstream plans and gates against the
  JSON, but humans/adjudications read the prose note; divergence makes
  audit evidence untrustworthy.
- **Status:** FIXED — CA-1236 (2026-10-08): CMakeLists.txt/*.cmake reclassified doc->code in IsDocOrAuditFile; freeze emits freeze_summary_mismatch diag when draft paths drop; frozen event carries declared_paths

## Evidence chain (all live)

1. run-523131 (vibe-sprint CP-11), sprint 1 `preflight_contract_freeze`:
   the freeze note asserted `core/vault-core/src/main/cpp/CMakeLists.txt`
   was declared, but the stored contract JSON (`frozen_contracts.ndjson`
   entry `c86945a3`) did not list it — freeze text and contract diverged.
2. Downstream contract-planner turns cited the frozen contract by the
   note's prose, while `r-code-drift`/scope checks evaluated the JSON —
   two sources of truth for the same scope.

## Root cause (hypothesis)

The freeze node writes a human-facing summary separately from the
contract object; nothing asserts the two are isomorphic. A prose path
list is free text — the contract store is the only gate input.

## Fix direction

- `F-1` Generate the freeze summary FROM the stored contract object
  (single source), or add a post-freeze check that every path named in
  the note exists in `declared_paths` — mismatch → block freeze.
- `F-2` Emit `freeze_summary_mismatch` drift event when they diverge.

## Regression coverage

- `TestBug644_FreezeSummaryMatchesContract` — freeze a contract with N
  paths; assert summary ↔ `declared_paths` set equality.
- `TestBug644_MismatchBlocksFreeze` — injected divergence → freeze fails.
