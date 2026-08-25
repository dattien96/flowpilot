# CA-640 — contract.freeze ignores FlowPilot's own runtime state (run-243681 F2 loop)

## What

CP-43 F2 live (run-243681, rag-harness): flow stuck at `preflight_contract_freeze`
WAITING_USER_APPROVAL in an infinite "ask continue" loop. The freeze step's
planner-mutation guard kept reporting:

```
flow_contract_freeze_blocked: "planner changed 2 file(s)
(.flowpilot/ledger/chat_summary.ndjson, .flowpilot/manifest.json);
the contract planner must be read-only"
```

## Why

`runContractFreezeNode` verifies the read-only planner made no project mutations
by diffing `baselineWorktreeFingerprint` (captured at flow start) against a fresh
fingerprint at freeze time. But `baselineWorktreeFingerprint` fingerprinted EVERY
dirty path from `git status`, including FlowPilot's **own** runtime metadata
(`.flowpilot/ledger/chat_summary.ndjson`, `.flowpilot/manifest.json`) — which the
runner itself rewrites on every turn. So each Continue re-ran the planner, the
runner persisted chat-summary/manifest, and the freeze saw them "changed" →
blocked → re-parked → infinite loop.

The gate's scope path already treats `.flowpilot/**` as "runtime metadata, not
product code" (`flowgate/observe.go` `IsDocOrAuditFile`); the freeze planner-
mutation check was missing the same exclusion. The GitNexus index (`.gitnexus/**`,
CA-639 auto-index) has the same property — it can be written mid-flow by the
background indexer.

## Fix

`apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`:

- New `isFlowPlannerExcludedPath(path)`: excludes `.flowpilot/**` and `.gitnexus/**`
  (runner-owned runtime state / tool-owned index).
- `baselineWorktreeFingerprint` skips excluded paths when capturing the baseline.
- `worktreeMutatedSincePaths` skips excluded paths when diffing (defense in depth).

Real project files (e.g. `calc.go`, `planner_touched.go`) are still captured and
still block — the guard is not weakened for genuine planner mutations.

Provider-agnostic (Case 1): no `ProviderKey` branch in the freeze dispatch.

Will not undo: CA-426 C-1 pre-existing-dirty allowance, CA-426 I-1 serialization,
CA-635/634 frozen-scope drift, CA-637/639.

## Tests

Additive only — `flow_contract_freeze_test.go` untouched.

- `apps/local-runner/internal/runner/run243681_freeze_planner_metadata_exclusion_test.go` (new):
  - `TestRun243681_FreezeIgnoresFlowpilotStateChanges` — exact repro: dirty
    `.flowpilot/ledger/chat_summary.ndjson` + `manifest.json` before flow start,
    rewritten between start and freeze → freeze succeeds, coder spawns.
  - `TestRun243681_FreezeStillCatchesPlannerProjectFileMutation` — `planner_touched.go`
    after flow start → still blocks, no coder spawn.
  - `TestWorktreeMutatedSincePathsIgnoresFlowpilotAndGitNexus` — pure diff, churn ignored.
  - `TestWorktreeMutatedSincePathsStillCatchesProjectFile` — `src/calc.go` change reported.
  - `TestBaselineWorktreeFingerprintExcludesFlowpilotAndGitNexus` — baseline skips both,
    still captures `real.go`.

## Verification

- `go test ./internal/runner -run 'TestRun243681|TestWorktreeMutatedSincePaths|TestBaselineWorktreeFingerprint|TestRunContractFreezeNode|TestInlineChain|TestRecoveredFlowReuses|TestDuplicateFreeze|TestFlowCoderUsesFrozenScope|TestFlowScopeDriftBlocks|TestFlowFrozen|TestFlowCoderComputesWrittenPaths' -count=1` → green.
- `go test ./internal/changecontract/... ./internal/flowgate/... -count=1` → green.
- `go vet ./internal/runner` clean.
- `TestRun147126_AuditHonorsFrozenContract` fails on macOS with `declared path resolves outside workspace via symlink` — pre-existing env failure, reproduced identically on the clean tree (verified before CA-637).

## Manual (operator tick)

Restart runner (load fix) → re-run F2: flow passes `preflight_contract_freeze`,
reaches coder, and now the real `FrozenContractScopeDrift` hard-block (the actual
F2 goal) can be exercised with a frozen scope missing `user_test.go`.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-43
change_type: bugfix
summary: contract.freeze planner-mutation guard ignores FlowPilot's own .flowpilot/.gitnexus runtime state so it stops false-blocking the freeze step (run-243681 F2 loop)
# --->8---
