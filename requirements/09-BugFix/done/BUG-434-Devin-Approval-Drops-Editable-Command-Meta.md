# BUG-434: Devin approval card drops command carried in `toolCall._meta["cognition.ai/editableCommand"]`

## Metadata

- Document ID: `BUG-434`
- Title: `devin exec permission carries command under _meta.editableCommand; extractor only reads title/rawInput → blank approval card`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-46-Test-Steps](../../07-Coding-Plan/done/), [BUG-374](BUG-374-Devin-Permission-ToolCallId-Only-Verdict-Gate-Wedge.md)
- Feature Keys: `provider-devin`, `agent-flow-engine`

## AI Quick View

### Summary

- `session/request_permission` for `rm -rf /tmp/lt46-del-dir` DID carry the command — inside `toolCall._meta["cognition.ai/editableCommand"]` — but `devinApprovalDetailsFromRequest` (`devin_adapter.go:922-947`) only reads `toolCall.title` and `toolCall.rawInput.{command,cmd,filepath,filePath,path}` → `ApprovalDetails{Command:"",Reason:""}` persisted. Approver sees an empty Command/Reason card and approves blind.
- Sibling of BUG-374 (same extractor, MCP-tool case where `toolCall` carries only `toolCallId`); this doc covers the exec case where devin sends the command under `_meta`.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom**: approval card for exec permission shows empty Command/Reason.
- **Expected**: card shows `rm -rf /tmp/lt46-del-dir` (present in `_meta.editableCommand`).
- **Actual**: `approvals.ndjson` appr-76 persisted with `Command:"", Cwd:"", Reason:""`; approve → `allow_once` round-trips but decision is blind.
- **Impact**: operators approve destructive commands without seeing them; defeats the point of the permission card.

## Reproduction

1. Devin chat in `smart` mode; prompt that triggers an exec (e.g. `rm -rf /tmp/…`).
2. `session/request_permission` arrives with `toolCall._meta["cognition.ai/editableCommand"]` set, `title`/`rawInput` absent.
3. Inspect `approvals.ndjson` — Command/Reason empty.

## Root cause

- `devin_adapter.go:922-947` — `devinApprovalDetailsFromRequest` doesn't read `toolCall._meta["cognition.ai/editableCommand"]` (nor `_meta` cwd/terminal fields).

## Evidence

- `~/fp-beds/lt-evidence/cp46/BUG-LIVE-CP46-R2-devin-approval-drops-editable-command.md`
- `~/fp-beds/lt-evidence/cp46/retest-runner.log` L205-207; `approvals.ndjson` appr-76; `r-appr76-decision.json`.

## Severity

medium

## Completion Notes (implemented 2026-09-22, CA-916)

- Root cause: exec permission requests carry the shell command under `toolCall._meta.cognition.ai/editableCommand`; the extractor only read title/rawInput → blank approval cards and lost the user-editable command surface.
- Fix: `devinApprovalDetailsFromRequest` reads `editableCommand` first (canonical surface), then correlated rawInput command fields, then title/toolName; correlated `tool_call` metadata restores title/kind for the card.
- Files: `internal/runner/devin_adapter.go`, `internal/runner/devin_event_mapper.go`.
- Tests: `bug_devin_toolcall_correlation_test.go`.
