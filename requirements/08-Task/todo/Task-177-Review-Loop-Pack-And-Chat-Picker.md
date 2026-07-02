# Task-177: Review Loop Pack And Chat Picker

## Metadata

- Document ID: `Task-177`
- Title: `Review Loop Pack And Chat Picker`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-01`
- Last Updated: `2026-07-01`
- Parent Documents: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- Child Documents: `Task-180`
- Related Documents: `Task-174`, `Task-175`, `Task-176`, `CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration`
- Replaces: `N/A`
- Tags: `chat-mode, review-loop, agent-flow-engine, builtin-flow`

## AI Quick View

### Summary

- Convert Chat Mode Review Loop from hardcoded runner semantics into a selectable built-in flow.
- Add Chat Mode UI control for optional built-in orchestration under the existing Chat sub-mode selector.
- Review Loop is offered only when Chat sub-mode is `bug`.
- Runtime selection is explicit via `flowRef`; Go still enforces join, cap, control, and auto-reinvoke.

### Current Ask

- Make Review Loop a read-only built-in flow option in Chat Mode Bug sub-mode using pack data and generic executor behavior.

### Key Decisions

- `T-1` RAG/context baseline remains always-on and is not the Chat Mode picker.
- `T-2` Review Loop is optional and scoped to Bug sub-mode because normal chat and task planning should not automatically spawn reviewer agents.
- `T-3` User can replace agent markdown content, but cannot break Go-enforced flow state transitions.

### Constraints

- Depends on `Task-174`, `Task-175`, and `Task-176`.
- Do not change Flow Mode custom authoring in this task.
- Preserve current Review Loop user-visible behavior.

### Open Questions

- Final Chat Mode picker copy can be refined during UI implementation.
- Whether Task sub-mode gets a different future built-in flow remains out of scope.

### Source Refs

- `apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml`
- `apps/local-runner/internal/agentpack/flow-pack/agents/coder.md`
- `apps/local-runner/internal/agentpack/flow-pack/agents/reviewer.md`
- `apps/local-runner/internal/agentpack/flow-pack/agents/synthesizer.md`

## 1. Goal

Expose Review Loop as a Chat Mode Bug sub-mode built-in flow selection and execute it through pack-backed flow definitions instead of role-specific runner branches.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- tech design: `requirements/06-System-Tech-Design/done/SD-19-Agent-Orchestration-Runtime.md`
- system spec: `requirements/05-System-Specs/done/SS-16-Agent-Orchestration.md`
- specific upstream ids: `CP-36`, `Task-174`, `Task-175`, `Task-176`

## 3. Trigger

Chat Mode already has `normal`, `bug`, and `task` sub-modes. Review Loop should be available as an explicit Bug sub-mode option because bug-fix work benefits most from code-review iteration, while normal chat and task planning should remain lightweight by default.

## 4. Exact Change

- `T-1` Add Chat Mode request fields for sub-mode and optional orchestration:
  - `subMode`
  - `flowRef` or `builtinFlowRef`
  - empty means normal chat.
- `T-2` Add UI section below the existing Chat sub-mode selector:
  - `Built-in orchestration`
  - options: `None`, `Review Loop`
- `T-3` Render the UI section only when current `subMode` has at least one matching built-in flow.
- `T-4` Populate options from built-in flow metadata where:
  - `selectableIn` contains `chat`
  - `chatBaseline != true`
  - `chatSubModes` contains the active `subMode`
- `T-5` For `subMode=bug`, show `None` and `Review Loop`.
- `T-6` For `subMode=normal` or `subMode=task`, hide the select unless future pack metadata declares a matching flow.
- `T-7` Switching away from Bug clears or disables an existing Review Loop selection.
- `T-8` When Review Loop is selected, send `subMode=bug` and `flowRef=flowpilot-core-flow-pack/review-loop` or canonical equivalent.
- `T-9` Runner resolves `flowRef` through `FlowDefinitionResolver`.
- `T-10` Review Loop nodes use pack agent definitions loaded by Task-174.
- `T-11` `submit_review_outcome` remains a declared control face but is mapped to generic `FlowControlInput`.
- `T-12` Replace Review Loop-specific Go branches with generic behavior dispatch where possible.
- `T-13` Add integration tests:
  - normal Chat Mode without picker does not spawn review agents.
  - Bug sub-mode shows Review Loop picker option.
  - Review Loop picker spawns coder then reviewers according to flow.
  - Normal and Task sub-modes do not show Review Loop.
  - switching from Bug to Normal clears the Review Loop selection.
  - `approved` maps to `done`.
  - `changes_requested` maps to `continue` and back-edge.
  - cap behavior still blocks/escalates.

## 5. Touched Areas

- files:
  - Chat Mode UI components
  - Chat Mode request contract/client
  - `apps/local-runner/internal/runner/interactive_service.go`
  - `apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml`
- modules:
  - desktop/web UI
  - local runner flow execution
  - provider bridge
- routes:
  - chat start/send endpoints that carry `flowRef`
- tables:
  - built-in mirrored flow definition rows

## 6. Acceptance Check

- Chat Mode default remains normal chat with RAG baseline only.
- Chat Mode Review Loop option appears from built-in metadata only for Bug sub-mode.
- Normal and Task sub-modes do not offer Review Loop.
- Selecting Review Loop sends explicit `flowRef`.
- Runner resolves and executes the pack-backed Review Loop.
- Existing CP-36 review-loop behavior is preserved.
- Tests prove user can change agent markdown without Go code edits.

## 7. Out of Scope

- Flow Mode custom editor.
- RAG Harness conversion.
- Removing all legacy review-loop fallback code.

## 8. Completion Notes

- result: implemented
- notes: `BuiltinOrchestrationOptions(subMode)` computes the Review Loop picker option from pack metadata ([CA-152](../../../change-audit/CA-152-chat-builtin-orchestration-options.md)), served over `GET /client/chat/builtin-orchestration-options`. `turnBody`/`TurnInput` carry `subMode`/`flowRef`, validated by `handleStartTurn` before a turn starts. The desktop's existing `ChatStartMode` panel (`"normal"|"task"|"bugfix"`, an earlier session pass wrongly claimed this didn't exist — see CA-156's correction) now has a "Built-in orchestration" picker sending `subMode="bug"`/`flowRef` on the first turn, clearing the selection whenever the intent changes away from Bug (T-7) — [CA-156](../../../change-audit/CA-156-chat-builtin-orchestration-picker-ui.md). **A selected `flowRef` now actually executes**: `startTurn`'s first-turn gate spawns the flow's entry node (`coder` for Review Loop) via the same `spawnChildRun`/`AutoOrchestrate=true` path an AI-driven `spawn_agent` tool call already uses, so all existing cohort/join/`isCoderRun`/hub-reinvoke/`flow_control` machinery drives the rest unmodified — [CA-158](../../../change-audit/CA-158-generic-flow-executor-entry-node-spawn.md).
- update: per explicit user direction, the hub-framing open question is resolved — a `prompts/flow-start-wait.md` pack prompt is now delivered to the hub as soon as the entry node is spawned, telling it an agent already started and to wait rather than redundantly do the coding itself. See [CA-160](../../../change-audit/CA-160-flow-definitions-migrated-to-workflows-table.md).
- follow-ups: Task-180's full `isCoderRun` elimination is unrelated to this and remains separately tracked.
- upstream docs updated: [CP-42](../../../07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) progress notes, [CA-152](../../../change-audit/CA-152-chat-builtin-orchestration-options.md), [CA-156](../../../change-audit/CA-156-chat-builtin-orchestration-picker-ui.md), [CA-158](../../../change-audit/CA-158-generic-flow-executor-entry-node-spawn.md), and [CA-160](../../../change-audit/CA-160-flow-definitions-migrated-to-workflows-table.md)
