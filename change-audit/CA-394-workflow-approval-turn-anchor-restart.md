# CA-394: Workflow restart keeps approval cards on their turn (run-35329)

## Summary

After a full server restart, reopening a Workflow/flow-engine run (live: Grok Review Loop `run-35329`, post-done follow-up) always rendered resolved `permission_required` cards at the **absolute bottom** of the timeline — after every later follow-up — instead of next to the prompt turn that requested the shell approval.

## Root cause

`reorderSidecarPrefixToEnd` restored durable gate events from the flow-events sidecar, then only called turn-anchoring when `runKind == "chat"`. Workflow hubs skipped anchoring and fell through to "move entire sidecar prefix after transcript", which pins every approval/question to the end.

## Fix

- Always attempt `anchorSidecarPrefixByTurnLocked` (renamed from chat-only helper) for every run kind.
- Fall through to timestamp reorder / end-append only when no turn ids match.

## Tests

- Additive `run35329_workflow_approval_turn_anchor_test.go`: Grok workflow full seed path + Claude/Codex/Grok unit matrix on reorder.
- Existing `TestRun2334NormalGrokRestartReplayUsesRawPromptsAndTurnAnchors` still green (chat path).

## Verification

```text
go test ./internal/runner -run 'TestWorkflowRestartAnchorsApproval|TestRun2334NormalGrokRestartReplay'
```

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: BUG-272
change_type: bugfix
summary: Anchor workflow/flow sidecar approvals to matching turn on restart so cards stay with their prompt (run-35329)
# --->8---
