# BUG-106: spawn_agent wait=true Returns Empty FinalMessage When Text Arrived Via EventMessageCompleted

## Metadata

- Document ID: `BUG-106`
- Title: `spawn_agent wait=true Returns Empty FinalMessage When Text Arrived Via EventMessageCompleted`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [Task-082: Spawn-Agent Tool And Orchestrator Core](../../08-Task/done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [BUG-105](BUG-105-Claude-Provider-Spawn-Agent-MCP-Silent-Degradation.md)
- Replaces: `None`
- Tags: `multi-agent, spawn-agent, wait, final-message, codex, regression`

## AI Quick View

### Summary

- When `spawn_agent` is called with `wait=true`, the parent receives only run metadata (`runId`, `providerKey`, `status`) — `finalMessage` is absent even though the child completed successfully.
- Root cause: `emitLocked` calls `signalChild(rs.id, ev.FinalMessage, ...)` using only the `EventTurnCompleted.FinalMessage` field. For Codex new-protocol (`turn/completed`) child runs, `EventTurnCompleted.FinalMessage` is empty because the actual content arrived earlier via `EventMessageCompleted` streaming events and `lastAgentMessageText` returned "" (no matching `agentMessage` item in the turn payload).
- `finalizeInputLocked` already has the correct fallback (`EventMessageCompleted.Text` when `EventTurnCompleted.FinalMessage` is empty), but `signalChild` did not share this logic.

### Current Ask

- In `emitLocked`, when handling `EventTurnCompleted`, fall back to the last `EventMessageCompleted.Text` in `rs.events` when `ev.FinalMessage` is empty before calling `signalChild`.
- Add `TestSpawnChildWaitReturnsFinalMessageViaMessageCompleted` to guard this path.

### Key Decisions

- `V-1` The fallback walks `rs.events` in reverse looking for the last `EventMessageCompleted` with non-empty `Text`. By the time `signalChild` is called, the current `EventTurnCompleted` has already been appended to `rs.events` (line 649 runs before the switch), so all prior events are available.
- `V-2` `rs.lastMessage` is NOT used as the fallback source because it is truncated to 100 characters via `truncateDisplayField` — inappropriate for a full child result.

### Constraints

- The fix is guarded by `finalMsg == ""` so it does not interfere with providers (Claude, Codex old-protocol) that already populate `EventTurnCompleted.FinalMessage` correctly.
- All existing spawn-agent tests continue to pass unchanged.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` — `emitLocked`, `finalizeInputLocked`
- `apps/local-runner/internal/runner/interactive_service_test.go` — new test
- `apps/local-runner/internal/runner/codex_event_mapper.go` — `mapCodexNotification`, `lastAgentMessageText`

## 1. Issue Summary

When the same prompt is sent via `spawn_agent` with `wait=true` (child Codex run, cross-provider spawn from a Claude parent), the response omits `finalMessage`:

```json
{"runId":"run-80","providerSessionId":"thread-81","providerKey":"codex","status":"completed"}
```

The child run status is `"completed"` (the child finished successfully), but the child's reply text is absent. Claude then reports: *"The spawn_agent call completed, but the tool response did not include the child's reply text — it returned only run metadata."*

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- tech design: [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- system spec: Task-082 is the originating task for `spawn_agent wait=true`

## 3. Environment and Reproduction

- environment: Desktop app, cross-provider spawn (Claude parent → Codex child), any platform
- reproduction steps:
  1. Start a Claude session in FlowPilot.
  2. Send: `Use spawn_agent with agent="reviewer", provider="codex", wait=true. Child prompt: "Do not use tools. Reply exactly: CHILD_AGENT_DONE."`
  3. Observe the returned JSON has `status: "completed"` but no `finalMessage`.
  4. The same prompt sent with a Codex parent works because Codex's old-protocol `turn.completed` carries `finalMessage` directly.
- frequency: Consistent for child runs that use the Codex new-protocol `turn/completed` notification

## 4. Expected vs Actual

- expected: `SpawnAgentResult.FinalMessage = "CHILD_AGENT_DONE"` (the child's reply text).
- actual: `SpawnAgentResult.FinalMessage = ""` (absent from the JSON response due to `omitempty`).

## 5. Impact

- users affected: All users using `spawn_agent wait=true` with a Codex child run (cross-provider or same-provider)
- workflows affected: Any multi-agent workflow where the parent reads the child's result inline
- severity: High — the feature (`wait=true`) is broken; the child's reply is silently lost

## 6. Root Cause

- hypothesis: `signalChild` only uses `EventTurnCompleted.FinalMessage`; when that field is empty the completion carries no text.
- confirmed cause: `emitLocked` in `interactive_service.go`:
  ```go
  s.agentOrchestrator.signalChild(rs.id, ev.FinalMessage, false, "", RunStatusCompleted)
  ```
  `ev.FinalMessage` comes from `mapCodexNotification` which for `turn/completed` calls `lastAgentMessageText(p["turn"])`. If the notification's `turn.items` does not contain an `agentMessage` entry (e.g. the child is a single-message run with no tool calls), `lastAgentMessageText` returns "". The actual text reached the runner as a prior `EventMessageCompleted` event, but `signalChild` doesn't look there.
  
  By contrast, `finalizeInputLocked` does have the correct fallback:
  ```go
  case EventMessageCompleted:
      if in.FinalMessage == "" {
          in.FinalMessage = e.Text
      }
  ```
  — but `finalizeInputLocked` is only used for the non-wait path (turn persistence); `signalChild` was not using the same logic.

## 7. Fix Strategy

- `F-1` In `emitLocked` when handling `EventTurnCompleted`, compute `finalMsg` with fallback:
  ```go
  finalMsg := ev.FinalMessage
  if finalMsg == "" {
      for i := len(rs.events) - 1; i >= 0; i-- {
          if rs.events[i].Type == EventMessageCompleted && rs.events[i].Text != "" {
              finalMsg = rs.events[i].Text
              break
          }
      }
  }
  s.agentOrchestrator.signalChild(rs.id, finalMsg, false, "", RunStatusCompleted)
  ```
- `F-2` Add `msgOnlyTurnAdapter` helper and `TestSpawnChildWaitReturnsFinalMessageViaMessageCompleted` test that verifies `FinalMessage` is populated from `EventMessageCompleted` when `EventTurnCompleted.FinalMessage` is "".

## 8. Validation

- `V-1` `TestSpawnChildWaitReturnsFinalMessageViaMessageCompleted` passes (new test guards the exact bug path).
- `V-2` `TestSpawnChildRunWaitTrueWaitsThroughApprovalGate`, `TestSpawnChildRunEmitsParentGraphWhenChildWaitsApproval`, `TestSpawnChildRunWaitTrueWaitsThroughQuestionGate`, `TestChangesRequestedRestartsCoderTurn`, `TestSpawnChildEmitsGraphAndBusEvents`, `TestAgentGraphRoutesExposeSnapshotAndControls` all continue to pass.

## 9. Regression Guard

- tests: `TestSpawnChildWaitReturnsFinalMessageViaMessageCompleted` uses `msgOnlyTurnAdapter` (emits only `EventMessageCompleted`, then empty `EventTurnCompleted`) and asserts `FinalMessage == "CHILD_AGENT_DONE"`.
- alerts: None.
- audit checks: `emitLocked` and `finalizeInputLocked` now share the same FinalMessage fallback logic.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — behavior contract for `wait=true` is unchanged; the fix makes the implementation match the documented contract.
- notes left unchanged on purpose: `lastAgentMessageText` in `codex_event_mapper.go` is not changed; the fallback is applied at the consumer (emitLocked) rather than the mapper to avoid affecting other code paths.
