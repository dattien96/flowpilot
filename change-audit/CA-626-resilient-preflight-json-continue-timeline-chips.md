# CA-626: Resilient Preflight Draft JSON Parsing, Continue Flow Fallback, and Timeline Action Chips

<!-- flowpilot:change-ledger -->
```yaml
schema_version: 1
change_id: CA-626
feature_key: change-contract
parent_change_id: CA-625
date: 2026-08-24
author: antigravity
summary: Extract embedded JSON from prose/markdown in ParsePreflightDraft, fallback to planner child draft on /continue freeze, and render interactive action chips on chat timeline
intent: Fix contract freeze parsing failure when models stream leading commentary, prevent invalid character 'c' on continue flow, and display action chips in the chat timeline
declared_paths:
  - apps/local-runner/internal/changecontract/preflight.go
  - apps/local-runner/internal/changecontract/preflight_test.go
  - apps/local-runner/internal/runner/flow_validate_audit_dispatch.go
  - apps/local-runner/internal/runner/freeze_continue_resilience_test.go
  - apps/local-runner/internal/runner/flow_contract_freeze_test.go
  - apps/local-runner/internal/runner/gate_hook.go
  - apps/local-runner/internal/tui/app/app.go
  - apps/local-runner/internal/tui/app/attention.go
  - apps/local-runner/internal/tui/app/mouse.go
  - apps/local-runner/internal/tui/app/tui_timeline_action_chips_test.go
```
<!-- /flowpilot:change-ledger -->

## Context & Problem

1. In `run-214858` (Rag Harness with Grok 4.5), the planner agent streamed leading commentary (`"I'll locate format.go..."`) before calling tools and then outputted the JSON draft. Because `ParsePreflightDraft` previously decoded the full raw string strictly with `json.NewDecoder`, it failed on the leading `'I'` with `invalid character 'I' looking for beginning of value`, causing an unwanted escalation to `WAITING_USER_APPROVAL`.
2. When the user clicked `[Continue]` on the escalated freeze step, the runner passed `feedback = "continue"` (or `"/continue"`) as `plannerResult` to `runContractFreezeNode`. The decoder failed again on `'c'` from `"continue"`, re-escalating the flow.
3. Interactive action chips (`[Continue] [Stop]`, `[Approve] [Deny]`, and `ask_user` questions) were previously packed into the bottom composer input box, cluttering the input area.

## Changes Made

1. **Resilient Preflight Draft JSON Parsing (`preflight.go`)**:
   - Added `extractJSONObject` to extract top-level JSON `{...}` while stripping surrounding markdown fences (````json ... ````) or conversational prose.
   - `ParsePreflightDraft` tries strict decoding first, and if that fails, extracts the embedded JSON object before returning any errors.
2. **Planner Result Fallback on Continue (`flow_validate_audit_dispatch.go`)**:
   - Added `findPlannerResultForFreeze` which resolves the predecessor planner child run's `finalMsg` when `plannerResult` is unparseable or contains `/continue` feedback.
3. **Interactive Action Chips in Chat Timeline (`app.go`, `mouse.go`, `attention.go`)**:
   - Updated `buildChatRows()` to render interactive action cards (Approval, Question, Attention, Blocked bar) directly at the end of the chat message stream.
   - Updated `chatRowsSig()` to hash approval, question, blocked, and attention state.
   - Removed action bars from `renderInputLine()` so the composer input box remains clean.
   - Updated `hitApprovalChrome`, `hitBlockedChrome`, `hitQuestionChrome`, and `hitAttentionChip` to hit-test against visible chat timeline rows (`sliceChatRows`).

## Validation

- `go test ./internal/changecontract/...`: PASSED.
- `go test ./internal/runner/ -run="TestContractFreeze_"`: PASSED across prose-wrapped planner draft and `/continue` child resolution.
- `go test ./internal/tui/app/ -run="TestTimelineActionChips_|TestTUIInput_"`: PASSED across `claude`, `codex`, and `grok`.
