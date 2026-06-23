# BUG-126: Tool-Spawn Wait=False Result Not Delivered To Parent Conversation

## Metadata

- Document ID: `BUG-126`
- Title: `Tool-Spawn Wait=False Result Not Delivered To Parent Conversation`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [BUG-122: UI-Spawn Children Invisible To Parent Provider Conversation](./BUG-122-UI-Spawn-Children-Invisible-To-Parent-Provider-Conversation.md), [Task-082: Spawn-Agent Tool And Orchestrator Core](../../08-Task/done/Task-082-Spawn-Agent-Tool-And-Orchestrator-Core.md), [CA-117: Fix Tool-Spawn Wait-False Result Delivery](../../../change-audit/CA-117-fix-tool-spawn-wait-false-result-delivery.md)
- Replaces: `None`
- Tags: `multi-agent, spawn, wait-false, provider-context, symmetry, runner`

## AI Quick View

### Summary

- A background (`wait=false`) child spawned by the AI `spawn_agent` tool returns only a `status: spawned` ack; its eventual result is never delivered to the parent's provider conversation.
- BUG-122 injected UI-spawn results into the parent's next turn but gated that injection to `uiInitiated`, so tool-spawned children were excluded entirely.
- Result: asking the main chat about a background tool-spawned agent's outcome returns nothing, while the equivalent UI spawn works — breaking the "tool and UI spawn behave the same" expectation and blocking the future auto-talk-agents mode.
- Fix: inject a child's completion/failure result whenever the result was NOT already returned synchronously — i.e. for UI spawns (any wait) and tool spawns with `wait=false`. Tool spawns with `wait=true` are unchanged (their result is the tool result).

### Current Ask

- Make tool-spawned and UI-spawned children deliver their final result to the parent conversation the same way, for both wait modes.

### Key Decisions

- `V-1` A child's result is injected into the parent's next provider turn when `uiInitiated || !waitForResult`.
- `V-2` A tool spawn with `wait=true` must NOT be injected (already returned synchronously as the tool result) — no duplication.
- `V-3` A tool spawn with `wait=false` must inject its completion result so the parent learns the outcome.
- `V-4` UI spawns (any wait) keep injecting, exactly as BUG-122.

### Constraints

- Only the final result is shared, never the child's full transcript/tool detail — agents stay isolated by design.
- No duplicate delivery: the injection must not re-send a result already carried by a synchronous tool result.
- Reuse the BUG-122 `pendingAgentContext` buffer and persistence; do not add a second mechanism.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` — `interactiveRun.waitForResult`, `spawnChildRun` stamping, `emitLocked` completion/failure injection conditions.
- `apps/local-runner/internal/runner/interactive_service_test.go` — `TestToolSpawnWaitTrueDoesNotInjectParentContext`, `TestToolSpawnWaitFalseInjectsResult`.

## 1. Issue Summary

When the parent AI calls `spawn_agent` with `wait=false`, `spawnChildRun` returns immediately with `status: spawned` and no final message. The child runs in the background. When it completes, its result is recorded on the child's own run but nothing is added to the parent's provider conversation. BUG-122's injection — which would carry the result to the parent — only fires for `uiInitiated` spawns, so tool-spawned background children are silently excluded. The parent can never report the outcome of a background agent it launched via the tool.

## 2. Parent Links

- impacted coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md) — P-2 single spawn path; this bug extends "tool and UI parity" to include eventual result delivery for background spawns.
- impacted tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md) — the new result-sharing section documents this delivery rule.
- impacted system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: Desktop FlowPilot local runner, any provider.
- reproduction steps:
  1. In a main chat, have the AI call `spawn_agent` with `wait=false` (a background sub-agent).
  2. Wait for the child to complete.
  3. Ask the main chat: "what did the background agent return?"
  4. Observe: the main chat has no record of the result.
  5. Compare: the same spawn via the UI panel (`wait=false`) does surface the result on the next turn.
- frequency: always, for tool spawns with `wait=false`.

## 4. Expected vs Actual

- expected: regardless of spawn entry point, after a child completes the parent can report its final result on the next turn.
- actual: tool-spawned `wait=false` children deliver nothing to the parent; only UI spawns and `wait=true` tool spawns work.

## 5. Impact

- users affected: anyone using background (`wait=false`) tool spawns; the future auto-mode where the orchestrator launches agents and reads their results asynchronously.
- workflows affected: multi-agent orchestration / auto-talk-agents.
- severity: Medium — no data corruption, but breaks the intended spawn symmetry and the foundation for auto agent coordination.

## 6. Root Cause

- hypothesis: the result-injection introduced by BUG-122 was scoped too narrowly.
- confirmed cause: in `emitLocked`, the completion and failure notes were appended only `if rs.uiInitiated`. Tool spawns have `uiInitiated=false`, and `wait=false` tool spawns also never return the result synchronously, so the result reached neither the tool result nor the injected context.
- evidence: `SpawnAgentResult` for `wait=false` carries `Status: "spawned"` with empty `FinalMessage`; the completion path appended context only for `uiInitiated`; `TestToolSpawnWaitFalseInjectsResult` fails before the fix and passes after.

## 7. Fix Strategy

- `F-1` Add `interactiveRun.waitForResult`, set from `SpawnAgentInput.Wait` at child identity stamping in `spawnChildRun`.
- `F-2` Change the completion-note and failure-note conditions in `emitLocked` from `if rs.uiInitiated` to `if rs.uiInitiated || !rs.waitForResult`, so UI spawns (any wait) and tool spawns with `wait=false` inject, while tool spawns with `wait=true` (already returned synchronously) do not.
- `F-3` Keep the spawn "started" note gated to `UIInitiated` only — a tool spawn's existence is already in provider history via the tool call; only its eventual result was missing.

## 8. Validation

- `V-1` `TestToolSpawnWaitFalseInjectsResult` — tool `wait=false` child result is injected into the parent's next provider turn. Pass.
- `V-2` `TestToolSpawnWaitTrueDoesNotInjectParentContext` — tool `wait=true` child result is NOT injected (no duplication). Pass.
- `V-3` `TestUISpawnInjectsContextIntoParentProviderTurn` — UI spawn injection unchanged. Pass.
- `V-4` `go build ./internal/runner/...` clean; targeted spawn/agent suite green except pre-existing environment failures unrelated to this change (skills-merge provider-home tests; a Windows-only file-rename failure in the separate BUG-124 migration test).

## 9. Regression Guard

- tests: `TestToolSpawnWaitFalseInjectsResult`, `TestToolSpawnWaitTrueDoesNotInjectParentContext`, `TestUISpawnInjectsContextIntoParentProviderTurn` in `interactive_service_test.go`.
- alerts: a background tool spawn whose result never appears in the parent's next turn indicates the condition regressed.
- audit checks: injection fires iff `uiInitiated || !waitForResult`.

## 10. Follow-Up Document Updates

- upstream docs that must change: SD-16 gains a result-sharing section (Section 15) and result-sharing tests (Section 14) describing this delivery rule.
- notes left unchanged on purpose: only the final result is shared; per the multi-agent design each agent remains isolated from the others' working detail.
