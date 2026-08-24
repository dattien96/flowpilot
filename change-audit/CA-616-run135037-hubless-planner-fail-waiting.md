# CA-616: Hub-less planner fail parks blocked (run-135037) + inherits hub model

## What

Run-135037 rag-harness (grok-4.5): entry `preflight_contract_plan` (contract-planner) spawned with `gpt-5.4` → Codex ChatGPT 400 `The 'gpt-5.4' model is not supported when using Codex with a ChatGPT account.` then step `[x] FAILED preflight_contract_plan reason: (no detail from runner)` and `Thinking 2m05s` + `2m05s` kept rising. No `[Continue]/[Stop]`.

## Why

1. **Model seed:** `resolveFlowNodeModel` returned seeded `step_definitions` `gpt-5.4` for planner → `providerKeyFromModel` Codex, even though session was grok-4.5. YAML has no model; seed wins.
2. **Thinking ghost:** `notifyHubOfFlowChildFailureLocked` did `step FAILED + reinvoke hub`. Rag-harness has `hub.inline == ""`, so reinvoke's `shouldParkHubWriteTurn` saw `child.turnInFlight` still true → `hub_parked`, 3 retries then stuck. Loop `rejected` ≠ `blocked` → F-0 `hub_stalled` refused, `workIsLive` stayed true. `RejectionNote` was empty so TUI showed `no detail`.
3. **Park leak:** `hasActiveFlowChild` counted a terminal Failed child as active when `turnInFlight` lingered.

## Fix

- **P1 inherit (shim, no CA-230/239/241 change):** `delegateSpawnModel` returns `""` for `contract-planner` / `preflight_contract_plan` → `spawnChildRun` inherits hub model/provider. `resolveFlowNodeProviderModel` same — F2 posture now shows `grok-4.5` etc. Other nodes (coder/reviewer) keep their own `gpt-5.4`.
  - `flow_executor.go` 3 spawns + `interactive_service.go` + `flow_validate_audit_dispatch.go` 2 spawns use shim.
- **P5 reason:** `setFlowStepFailedWithReason{,Locked,Core}` stores `RejectionNote` on FAILED so `formatStepChatLine` surfaces the 400 text.
- **P3 leak:** `hub_stall.go hasActiveFlowChild` skips `Failed/Completed/Cancelled` even if `turnInFlight` lingers.
- **P2 hub-less park (new, preserves CA-355):** in `notifyHubOfFlowChildFailureLocked`, if `hub.inline == ""` → `lastFailedDelegateNodeID = label`, `mutateLoop blocked/delegate_failed` with `GateReason=err`, `parkFlowForAwaitingUserLocked` + `emitAgentGraphLocked`, no reinvoke. Review-loop with hub keeps CA-355 `reinvoke hub`.
- **P4 Continue:** `resumeFlowWithFeedback` replays `lastFailedDelegateNodeID` via `reinvokeMatchingFlowChild` (reset to RUNNING) else fallback spawn with `delegateSpawnModel`; clears the remembered id. `parkFlowForAwaitingUserLocked` avoids deadlock (emitLocked holds `s.mu`).
- Provider-agnostic: no `ProviderKey` branch. Verified by matrix `grok/codex/claude` in new tests + `grep -R ProviderKey flow_executor.go flow_step_runtime.go hub_stall.go` 0 hits on new symbols.

Will not undo: CA-355 review-loop reinvoke; CA-230/239/241 resolve model; CA-587 audit WAITING; CA-610–615 paste/clipboard.

## Tests (additive, no old edit)

- `run135037_planner_inherits_hub_model_test.go`: matrix grok/codex/claude inherits; non-planner reviewer keeps `gpt-5.4`.
- `run135037_hubless_planner_fail_waiting_test.go`: (4) grok/codex/claude parks `blocked/delegate_failed` with `RejectionNote`+`GateReason` `gpt-5.4` and no hub reinvoke + `hasActiveFlowChild` false; pre-adapter start-fail also parks; review-loop with hub still arms reinvoke (CA-355); `Continue` reinvokes planner → RUNNING + loop running.
- `run135037_tui_failed_reason_and_chips_test.go` (tui/app, table 3 providers): `formatStepChatLine` surfaces `gpt-5.4` from `RejectionNote`; `blocked/delegate_failed` renders `[Continue]/[Stop]` and `!workIsLive`.

Old suites: `go vet ./internal/runner ./internal/tui/app` pass; `go test ./internal/runner -run TestRun135037|TestNonCohortEntryFail|TestRun43831|TestResolveFlowNodeModel|TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel|TestSetFlowStepAwaitingUser` pass; `go test ./internal/tui/app -run TestBlockedBar|TestRun127174|TestRun135037` pass.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-43
change_type: bugfix
summary: hub-less planner inherits hub model and fails to blocked/delegate_failed with Continue/Stop (run-135037)
# --->8---
