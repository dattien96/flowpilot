# BUG-436: Devin `tool_call_update status:in_progress` maps to client `tool_completed` with `toolName:"tool"`

## Metadata

- Document ID: `BUG-436`
- Title: `progress ticks masquerade as tool_completed; tool title dropped to "tool"`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-46-Test-Steps](../../07-Coding-Plan/done/), [BUG-375](BUG-375-Devin-FileChanged-Never-Emitted.md)
- Feature Keys: `provider-devin`, `chat-replay`

## AI Quick View

### Summary

- Every devin `session/update` `tool_call_update` — including `status:"in_progress"` ticks — is emitted to client SSE as `type:"tool_completed"`, and most carry fallback `toolName:"tool"` instead of the update's `title`/`inferenceToolName`. Real completions (`status:"success"`) are indistinguishable in type from progress frames.
- Same mapper family as BUG-375 (`devin_event_mapper.go`) — mutation/status metadata consistently dropped or mis-shaped.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom**: `tool_completed` events fire during tool progress with `status:"in_progress"` and generic `toolName:"tool"`.
- **Expected**: progress → a progress/update event (or suppressed); `tool_completed` only on terminal status; `toolName` from `title`/`inferenceToolName`.
- **Actual**: `tool_started toolName="Ran rm"` → 3× `tool_completed status:in_progress toolName:"tool"` → `tool_completed status:success`. Same on MCP calls (`mcp_list_tools` progress → `tool_completed`).
- **Impact**: event-vocabulary fidelity — clients/UIs counting tool completions see phantom completions; useful titles/cwd/terminal_exit metadata dropped.

## Reproduction

1. Devin turn invoking any long-running tool (exec or MCP).
2. Watch client SSE: `tool_call_update` frames with `status:"in_progress"` arrive as `tool_completed`.

## Root cause

- `devin_event_mapper.go` tool_call_update → client event mapping ignores `status` and prefers neither `title` nor `inferenceToolName` → `tool_completed` + `toolName:"tool"`.

## Evidence

- `~/fp-beds/lt-evidence/cp46/BUG-LIVE-CP46-R4-tool-completed-on-progress.md`
- `~/fp-beds/lt-evidence/cp46/r-run38-resume-events.sse` evt-78/79/80, evt-111-113; `retest-runner.log` L208-211.

## Severity

low

## Completion Notes (implemented 2026-09-22, CA-916b)

- Root cause: `mapDevinToolCallUpdate` emitted `tool_completed` for ANY non-empty status — live `in_progress` ticks double-fired the lifecycle and could mark a still-running mutation done (also dropped titles to "tool").
- Fix: `devinToolCallStatusTerminal` gate — `in_progress`/`pending`/`running`/`queued`/empty consume the update without emitting; terminal statuses map normally; unknown statuses keep legacy emit (safer than dropping a completion). Correlation (BUG-375 index) restores the real title on terminal updates.
- Files: `internal/runner/devin_event_mapper.go`.
- Tests: `bug_devin_toolcall_correlation_test.go`.
