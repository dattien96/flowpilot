# CA-586: Fix validate skipped when parent Completed but loop still running (run-123336)

## What

Rag-harness F1 run-123336 hung after `implement` DONE: `validate`/`audit` stayed PENDING, TUI polled every 1.6s, hub stalled 2m then runner died. Log showed `flow_inline_dispatch_skipped_terminal` for `validate` even though `loop_status=running`, `loop_mode=explicit`.

## Why

- Parent run `run-123336` was `Completed` early (chat kickoff without file changes, `markPendingFlowGateSettle` not armed) while explicit loop was still `running`. This is the normal early-Completed case for flow kickoff.
- `flowRunTerminalLocked` and `flowInlineContext` in `flow_validate_audit_dispatch.go:106,137` treated **any** `Completed` as terminal (BUG-288 P1-12 Stop guard). `tryAdvanceFlowThroughInline` then returned `true` via `flow_inline_dispatch_skipped_terminal` before `runValidateNode`, so validate never ran.
- `flowInlineContext` also returned a pre-cancelled context, so even if the skip were removed the suite would be cancelled.

## Fix

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:106` `flowRunTerminalLocked`: split `Completed` from `Failed/Cancelled`. `Completed` + `loopIsAdvancing==true` → **not** terminal (return false). `Cancelled`/`Failed` remain terminal even when loop running (BUG-288).
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:142` `flowInlineContext`: same split; `Completed` + loop advancing breaks to live context instead of `alreadyCancelledContext`.
- Provider-agnostic: no `ProviderKey` branching (loop+status only). Grep `ProviderKey` in file = 0 hits.

Will not undo: BUG-288 P1-12 (Stop/Cancelled still skips), CA-585 (validate RUNNING stamp before `RunValidationCommand`).

## Tests

- New `run123336_flow_parent_completed_loop_running_test.go` (additive, no old edit):
  - `TestFlowRunTerminalLocked_ParentCompletedLoopRunning_NotTerminal` (provider-agnostic, Codex probe)
  - `TestFlowRunTerminalLocked_ParentCompletedLoopDone_Terminal`
  - `TestFlowRunTerminalLocked_Cancelled_AlwaysTerminalEvenIfLoopRunning`
  - `TestFlowInlineContext_ParentCompletedLoopRunning_Live`
  - `TestFlowInlineContext_ParentCompletedLoopDone_Cancelled`
  - `TestFlowInlineContext_Cancelled_AlwaysCancelledEvenIfLoopRunning`
  - `TestTryAdvanceFlowThroughInline_ParentCompletedLoopRunning_NotSkipped` (contrasts Cancelled true vs Completed+running not terminal)
- Old suite untouched and green: `go test ./internal/runner -run "TestFlowInlineContextReturnsCancelled|TestTryAdvanceFlowThroughInlineSkipsTerminalRun|TestFlow"` pass; `go vet` clean.
- Cross-provider parity: fix is agnostic; same loop/status path for claude/codex/grok. Probe uses Codex (claude/grok controlled runtime not in test server).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-56
change_type: bugfix
summary: allow validate/audit inline dispatch when parent Completed but loop still advancing (run-123336)
# --->8---
