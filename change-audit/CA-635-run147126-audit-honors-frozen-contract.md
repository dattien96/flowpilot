# CA-635: run-147126 — audit tier-3 honors FrozenStore as declared contract

<!-- flowpilot:change-ledger -->
```yaml
schema_version: 1
change_id: CA-635
feature_key: change-contract
parent_change_id: CA-634
date: 2026-08-25
author: main
summary: Audit tier-3 treats an active FrozenStore contract as ContractDeclared so rag-harness no longer loops r-contract on Continue
intent: Stop run-147126 synthesis<->audit loop caused by audit reading only the legacy Store which a frozen writer deliberately skips
declared_paths:
  - apps/local-runner/internal/changecontract/preflight.go
  - apps/local-runner/internal/runner/flow_validate_audit_dispatch.go
  - apps/local-runner/internal/runner/interactive_service.go
  - apps/local-runner/internal/runner/run147126_audit_honors_frozen_contract_test.go
  - change-audit/CA-635-run147126-audit-honors-frozen-contract.md
```
<!-- /flowpilot:change-ledger -->

## Context & Problem (run-147126, rag-harness grok-4.5)

1. Preflight froze `format.go` + `format_test.go` into `?? .flowpilot/contracts/frozen_contracts.ndjson`. The coder is deliberately `skipSave` (CP-55 P-8, CA-634) — the legacy `contracts.ndjson` Store is never written for a frozen writer.
2. Audit tier-3 (`runAuditNode`) read ONLY `changecontract.OpenStoreReadOnly().GetLatestForRun()` for `ContractDeclared`. Store empty → `false` → `r-contract` fired: `code changed without a declared Change Contract (used an inferred one) (tier-1 should have caught earlier)`.
3. Each Continue re-ran `audit` (inline-dispatchable), re-escalated with the same reason (`no progress since last continue`), and `resumeFlowWithFeedback` stamped `synthesis` (hub) RUNNING before routing — flapping the last two steps forever, never `DONE`.

## Changes Made

1. **F-1: `FrozenStore.ListForRun`** (`changecontract/preflight.go`): additive API returning every frozen record bound to a run (incl. superseded/abandoned) — audit needs it when live topology may not resolve.
2. **F-2: `frozenContractForRun` / `frozenContractDeclaredForRun`** (`runner/flow_validate_audit_dispatch.go`): an active frozen record for any `agent.code` writer node in the live topology counts as `ContractDeclared`; fallback scans `ListForRun` (restart-safe). Superseded/abandoned records do NOT count (`GetFrozenForStep` semantics).
3. **F-3: audit r-contract + FeatureKey** (`runAuditNode`): `ContractDeclared` = frozen OR legacy-declared; frozen FeatureKey also feeds the draft (mirrors run-127174 but from FrozenStore).
4. **F-4: Continue hub-stamp** (`interactive_service.go`): only stamp the hub `RUNNING` when the escalated node IS the hub or a pending hub prompt exists — a writer/audit park resume no longer flaps `synthesis`.

## Validation

- `go vet ./internal/runner ./internal/changecontract ./internal/flowgate` clean.
- New `run147126_audit_honors_frozen_contract_test.go` — 5 tests × provider matrix (grok/codex/claude):
  - `AuditHonorsFrozenContract` — legacy empty + active frozen → audit settles (done), no r-contract.
  - `AuditStillBlocksWithoutAnyContract` — no frozen, no legacy → still escalates r-contract.
  - `AuditStillHonorsLegacyDeclaredContract` — BUG-288 #2 legacy path unchanged.
  - `SupersededFrozenStillBlocks` — abandoned/superseded frozen does not count.
  - `ContinueAfterAuditParkDoesNotFlapSynthesis` — Continue after park settles, loop leaves `blocked`.
- Related old patterns green: `TestBUG327_*`, `TestRun144900_*`, `TestRun127174_*`, `TestTryAdvanceFlowFromNode*`, `TestRunAuditNode*`, `TestFlowCoder*`, `TestFrozen*`, `TestRAGHarness*`, `TestRun135037*` — 37.8s pass. `go test ./internal/changecontract ./internal/flowgate ./internal/agentpack` green.
- Provider parity: Case 1 agnostic — `grep providerKey` on changed production lines: only data-carrying field reads (no branch). Tests are provider-parameterized anyway.

## Will not undo

CA-634 (writer retry + leftover-skill drift), CA-627 (ledger exemption + stamp), BUG-288 #2 (legacy declared still honored), CP-55 P-8 `skipSave:true` design, CA-427 (no wholesale exemption).

## Residual

`run-147126` in-flight is still parked; user must `/stop` and start a new run with the rebuilt binary. Old runs with the same loop benefit only after restart.

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: BUG-288
change_type: bugfix
summary: Audit tier-3 treats an active FrozenStore contract as ContractDeclared so rag-harness no longer loops r-contract on Continue (run-147126)
# --->8---
