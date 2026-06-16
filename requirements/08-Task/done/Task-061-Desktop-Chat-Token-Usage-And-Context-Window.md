# Task-061: Desktop Chat Token Usage And Context Window

## Metadata

- Document ID: `Task-061`
- Title: `Desktop Chat Token Usage And Context Window`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [08-Desktop-Chat-New-Plan](../../10-Refactor/New-System/08-Desktop-Chat-New-Plan.md), [04-Detailed-Coding-Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md), [03-Solution-And-System-Design](../../10-Refactor/New-System/03-Solution-And-System-Design.md)
- Child Documents: `none`
- Related Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](./Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [Task-046: Desktop Chat Workspace Layout Rework](./Task-046-Desktop-Chat-Workspace-Layout-Rework.md), [Task-047: Desktop Chat Rail Polish And Resizing](./Task-047-Desktop-Chat-Rail-Polish-And-Resizing.md), [Task-049: Desktop Chat Provider Skills And Mode Regression Fix](./Task-049-Desktop-Chat-Provider-Skills-And-Mode-Regression-Fix.md)
- Replaces: `none`
- Tags: `desktop-chat, token-usage, context-window, codex, claude, runner-events`

## AI Quick View

### Summary

- Add runner-to-desktop token usage telemetry for Codex and Claude chat turns.
- Show a compact usage line in the desktop composer with context used, context remaining, and last-turn token counts.
- Keep the previous usage line visible while a new run is in progress, then replace it only when fresh usage telemetry arrives.

### Current Ask

- Done. Codex and Claude usage events are mapped into the shared contract, stored in desktop state, and rendered in the chat input with highlighted numeric values.

### Key Decisions

- `T-1` Normalize provider usage payloads into one `token_usage_updated` event shape before they reach the desktop client.
- `T-2` Keep the UI change scoped to the desktop chat surface and avoid altering broader run-state behavior beyond storing the latest token usage snapshot.
- `T-3` Preserve the last rendered usage line during the next run instead of replacing it with placeholder copy while waiting for new telemetry.

### Constraints

- Keep the feature limited to the desktop chat and runner event-mapping path.
- Do not redesign unrelated chat controls or timeline behavior.
- Preserve compatibility with providers that omit usage telemetry.

### Open Questions

- Gemini still has no matching token/context telemetry path in this slice.

### Source Refs

- `08-Desktop-Chat-New-Plan §6`
- `04-Detailed-Coding-Plan`
- `03-Solution-And-System-Design`

## 1. Goal

Expose per-turn token usage and model context-window telemetry in the desktop chat UI so the operator can see how much context the current provider turn consumed and how much room remains.

## 2. Parent Links

- coding plan: [08-Desktop-Chat-New-Plan](../../10-Refactor/New-System/08-Desktop-Chat-New-Plan.md); [04-Detailed-Coding-Plan](../../10-Refactor/New-System/04-Detailed-Coding-Plan.md)
- tech design: [03-Solution-And-System-Design](../../10-Refactor/New-System/03-Solution-And-System-Design.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `08-Desktop-Chat-New-Plan §6`

## 3. Trigger

The desktop chat plan explicitly called out `Show token/context`, with Codex already partially confirmed and Claude still pending. The operator also needed the desktop UI to retain the previous usage line during a new run instead of replacing it with a temporary waiting message.

## 4. Exact Change

- `T-1` Extend the shared runner/desktop event contract with `TokenUsageBreakdown`, `TokenUsageSnapshot`, and `token_usage_updated`.
- `T-2` Map Codex `thread/tokenUsage/updated` notifications and Claude usage payloads into the normalized event shape.
- `T-3` Store the latest token usage snapshot in desktop state and render a compact usage line in `ChatInput`.
- `T-4` Highlight numeric values in the usage line and keep the previous line visible until fresh telemetry arrives for the next run.
- `T-5` Add focused runner tests for Codex and Claude usage-event mapping and wire the desktop mock stream so the feature is visible in local mock mode.

## 5. Touched Areas

- files:
  - `apps/desktop-flowpilot/src/components/ChatInput.tsx`
  - `apps/desktop-flowpilot/src/state/store.ts`
  - `apps/desktop-flowpilot/src/styles.css`
  - `apps/desktop-flowpilot/src/types/contract.ts`
  - `apps/desktop-flowpilot/src/client/MockRunnerClient.ts`
  - `apps/desktop-flowpilot/src/client/mockData.ts`
  - `apps/local-runner/internal/runner/provider_event.go`
  - `apps/local-runner/internal/runner/codex_event_mapper.go`
  - `apps/local-runner/internal/runner/codex_event_mapper_test.go`
  - `apps/local-runner/internal/runner/claude_event_mapper.go`
  - `apps/local-runner/internal/runner/claude_adapter_test.go`
- modules: `desktop-flowpilot`, `local-runner`
- routes: `desktop chat composer`
- tables: `none`

## 6. Acceptance Check

- Codex usage telemetry maps from the provider stream into `token_usage_updated` events.
- Claude assistant/result usage payloads map into the same normalized event shape.
- Desktop chat shows context used, context remaining, and last-turn token counts when telemetry is available.
- All numeric values in the usage line use the highlight color.
- Starting a new run does not replace the previous usage line with placeholder waiting text; the line updates only after fresh telemetry arrives.
- `cd apps/desktop-flowpilot && npm run typecheck` passes.
- `cd apps/local-runner && GOCACHE=/private/tmp/flowpilot-gocache go test ./internal/runner -run 'Test(MapCodexNotification|MapClaudeLineAssistantToolUseAndFileChange|MapClaudeLineResultUsage)$' -count=1` passes.

## 7. Out of Scope

- No Gemini telemetry work in this task.
- No account-usage sidebar redesign or quota-management UI changes.
- No changes to workflow history, artifacts, or approval/question-card flows.

## 8. Completion Notes

- result: Token/context telemetry now flows from Codex and Claude into the desktop chat composer, with compact sticky display behavior across runs and highlighted numeric values.
- follow-ups: Add Gemini support if that provider begins exposing comparable usage/context telemetry.
- upstream docs updated: `Task-061`, `08-Desktop-Chat-New-Plan`
