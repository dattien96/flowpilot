# CA-259: Fall Back To Normal Turn When Explicit FlowRef Resolve Fails

## Scope

Fixed BUG-261, found live while diagnosing a stuck Chat Mode run (`run-10560`): an explicit chat `flowRef` (Bug sub-mode's Review Loop picker) whose stored flow definition failed to resolve left the run permanently stuck at `completed` with no reply and no error, because the hub's own turn was already unconditionally suppressed (`flowStartOnly=true`) before the async resolve failure was known. The sibling Flow-Mode workflow-picker path already resolved synchronously and never hit this gap.

## Changes

- `flow_executor.go`: added `explicitFlowRefResolves`, mirroring `resolveWorkflowFlowRef`'s existing resolve-before-attach pattern.
- `interactive_handlers.go`: `handleStartTurn` now calls `explicitFlowRefResolves` when the client sends a non-empty `flowRef`; on failure it logs and clears `body.FlowRef` before calling `startTurn`, so the turn falls through to a normal provider call instead of the flow-handoff short-circuit.

## Verification

- `go build ./...` — passed.
- `go vet ./internal/runner/` — passed.
- `go test ./internal/runner/ -run 'TestChatModeExplicitFlowRefResolveFailureFallsBackToNormalTurn|TestChatModeHandleStartTurnMarksRunFlowEngineDriven|TestStartTurnWithFlowRefEmitsSyntheticTurnCompletedForHubHandoff|TestStartTurnWithFlowRefSpawnsEntryNodeAsynchronously' -v -count=1` from `apps/local-runner` — all 4 passed (new regression test plus the 3 sibling tests confirming the successful-resolve path is unchanged).
- `go test ./internal/runner/... -count=1` — 1145 passed, 15 failed (pre-existing, environment-specific: missing Codex CLI, Windows path assertions, skill-precedence tests needing real files — same baseline as prior sessions), 14 skipped.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-261
change_type: bugfix
summary: resolve explicit chat flowRef synchronously and fall back to a normal turn when it fails, instead of permanently suppressing the hub's reply
# --->8---
