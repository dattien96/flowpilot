# CA-587: Rag-harness audit must show WAITING_USER when escalated without hub

## What

Run-125458 F1 ClampChecked: `validate` passed, `audit` built `blocked_missing_feature_key` and escalated (`loop blocked`), but F2/ chat still showed `[•] audit RUNNING` and `Thinking 9m`. No `[Continue]/[Stop]` bar, user saw infinite in-progress.

## Why

- `setFlowStepAwaitingUser` (`flow_step_runtime.go:219`) only stamped `hub.inline` (review-loop's `synthesis`). Rag-harness has no hub (`hubInlineNodeID` == ""), so the escalate was a no-op. `audit` stayed `RUNNING` even after `flow_parked_awaiting_user` / `hub_stalled`.
- TUI `workIsLive` already returns `false` when `flowLoopBlocked` is `blocked` and no child is live, but loop blocked alone was not enough to hide `RUNNING` — the step status itself was wrong. With correct WAITING status, `shouldPollStepsRuntime` and `workIsLive` settle and the blocked bar appears via `applyAgentGraph`'s `showBlockedBanner`.

## Fix

- `apps/local-runner/internal/runner/flow_step_runtime.go:219` `setFlowStepAwaitingUser`: keep hub path for review-loop, add fallback for flows without hub. If no hub, load run steps and mark the `RUNNING` node (audit) as `WAITING_USER_APPROVAL`; if no `RUNNING`, fallback to `audit` id. No provider branching (loop+status only).
- Provider-agnostic: no `ProviderKey` switch; `grep ProviderKey flow_step_runtime.go` 0 hits. TUI blocked handling (`step_runtime.go:162`, `app.go:2833`) is already provider-agnostic.

Will not undo: BUG-231 hub WAITING, CA-585 validate/audit RUNNING stamp before exec, CA-586 Completed+loop running.

## Tests

- New `run125458_rag_harness_audit_waiting_test.go` (additive, no old edit):
  - `TestSetFlowStepAwaitingUser_RagHarnessNoHub_MarksRunningAuditWaiting` — rag-harness nodes, audit RUNNING → WAITING_USER
  - `TestSetFlowStepAwaitingUser_ReviewLoopHubStillMarksHub` — review-loop hub still marked, coder not
  - `TestSetFlowStepAwaitingUser_RagHarnessNoRunning_FallbackToAudit` — no RUNNING, still marks audit
- Old suite untouched and green: `go vet ./internal/runner`, `go test ./internal/runner -run TestSetFlowStepAwaitingUser|TestFlow -count=1` pass; TUI `go test ./internal/tui/app -count=1 -p 1` pass (flaky `TestSlashAfterDraft` passes solo).
- Cross-provider: agnostic — single Codex probe plus grep evidence; claude/grok share same path.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-56
change_type: bugfix
summary: rag-harness audit escalate must mark WAITING_USER when no hub.inline (run-125458)
# --->8---
