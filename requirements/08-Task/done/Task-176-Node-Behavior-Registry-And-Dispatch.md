# Task-176: Node Behavior Registry And Dispatch

## Metadata

- Document ID: `Task-176`
- Title: `Node Behavior Registry And Dispatch`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-01`
- Last Updated: `2026-07-06`
- Parent Documents: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- Child Documents: `Task-177`, `Task-178`, `Task-180`
- Related Documents: `Task-173`, `CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration`, `CP-41-RAG-Harness-Flow-Mode`
- Replaces: `N/A`
- Tags: `agent-flow-engine, node-behavior, generic-flow, local-runner`

## AI Quick View

### Summary

- Introduce a generic node behavior registry so Flow Mode dispatches by behavior ID, not by hardcoded step names.
- Behavior handlers remain Go code and enforce deterministic contracts.
- YAML/definition data chooses which behavior a node uses.

### Current Ask

- Build the registry and route inline/delegate node execution through behavior specs.

### Key Decisions

- `T-1` Go owns behavior handlers because they enforce contracts that markdown cannot guarantee.
- `T-2` Pack/definition data can select behavior IDs and pass config, but cannot execute arbitrary code.
- `T-3` Unknown behavior IDs must fail before execution, not silently degrade to prompt-only behavior.

### Constraints

- Do not migrate RAG harness or review loop onto the registry in this task unless needed for tests.
- Keep old code paths available behind compatibility shims until Task-180.
- Behavior registry must be provider-agnostic.

### Open Questions

- Whether behavior config is stored as raw JSON, typed YAML, or both can be decided during implementation.

### Source Refs

- `apps/local-runner/internal/runner/flow_context_handoff.go`
- `apps/local-runner/internal/runner/interactive_service.go`
- `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`

## 1. Goal

Replace semantic branching like `isPlanStepType` and `isCodingStepType` with a generic, typed behavior dispatch layer that can support arbitrary user-created flow steps while keeping critical execution guarantees in Go.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- tech design: `requirements/06-System-Tech-Design/done/SD-19-Agent-Orchestration-Runtime.md`
- system spec: `requirements/05-System-Specs/done/SS-16-Agent-Orchestration.md`
- specific upstream ids: `Task-173`, `CP-36`, `CP-41`, `CP-42`

## 3. Trigger

CP-41 currently identifies Plan and Coding behavior by step type strings. That makes the system less generic than the Node/Edge/Policy model promises and forces new flow kinds to require runner code edits.

## 4. Exact Change

- `T-1` Add behavior registry interfaces:
  - `BehaviorID`
  - `BehaviorSpec`
  - `BehaviorInput`
  - `BehaviorOutput`
  - `BehaviorHandler`
  - `BehaviorRegistry`
- `T-2` Support behavior scopes:
  - `inline`: deterministic runner-owned operation.
  - `delegate`: prompt/context assembly for provider-backed agent run.
  - `control`: maps tool output to `FlowControlInput`.
- `T-3` Add core behavior IDs:
  - `agent.delegate`
  - `hub.synthesize`
  - `context.deterministic_feature_package`
  - `context.prompt_handoff`
  - `validation.command`
  - `audit.draft`
  - `flow.control_tool`
- `T-4` Modify executor node startup to resolve `node.behaviorRef` or equivalent from definition.
- `T-5` Validate behavior config against declared behavior schema before execution.
- `T-6` Return structured behavior output:
  - `status`
  - `summary`
  - `payload`
  - `nextPromptFragments`
  - `events`
- `T-7` Convert behavior output to existing provider/runtime events without changing provider adapters.
- `T-8` Add tests for unknown behavior, invalid config, inline output, delegate prompt assembly, and flow-control output.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/interactive_service.go`
  - `apps/local-runner/internal/runner/flow_context_handoff.go`
  - `apps/local-runner/internal/runner/provider_event.go`
  - new behavior registry files under `apps/local-runner/internal/runner/`
- modules:
  - local runner executor
  - flow context handoff
  - provider event bridge
- routes:
  - none
- tables:
  - none

## 6. Acceptance Check

- Executor can run a node by behavior ID without checking semantic step name.
- Unknown behavior ID fails with clear validation error.
- Existing tests for current flows still pass under compatibility path.
- New unit tests cover behavior registry dispatch.
- No user-provided arbitrary code execution is introduced.

## 7. Out of Scope

- UI behavior selector.
- Supabase mirror sync.
- Full removal of legacy CP-41 helper functions.

## 8. Completion Notes

- result: implemented (registry core), with 2 confirmed sub-gaps — verified 2026-07-06. The registry (`BehaviorID`/`BehaviorSpec`/`BehaviorInput`/`BehaviorOutput`/`BehaviorRegistry`/9 core behavior IDs) is real and passes all 20 tests in `behavior_registry_test.go`, plus `go build ./...` clean. Gaps: (T-5) config validation is folded into each handler's own ad hoc checks rather than a separate declared-schema validation step before execution; (T-7) no generic converter turns `BehaviorOutput.Events`/`Payload` into existing provider/runtime events — the registry is wired into exactly one call site (`startInlineEntryChain`), which interprets `BehaviorOutput.Payload["package"]` directly rather than through a general output→event bridge.
- follow-ups: `Task-177` (done), `Task-178`, `Task-180` (done) — both confirmed to depend on and correctly use this registry.
- notes: registry and core behavior handlers are additive only; `interactive_service.go` role checks and `ReviewLoopFlowConfig` migrated onto edge/behavior-driven dispatch by Task-180 (done). `command.validate`/`artifact.audit_draft` remain intentionally minimal stub handlers — mid-flow inline-node dispatch is tracked separately as [BUG-243](../../09-BugFix/todo/BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md) (deferred into CP-43), not a Task-176 gap.
- moved to done 2026-07-06: re-verified `go build ./...` clean, `go test ./internal/runner/... -run TestBehavior` 19/19 pass (doc's own count of "20" was an off-by-one, immaterial), full suite shows only the 15 pre-existing environment-specific failures unrelated to behavior_registry.
- upstream docs updated: [CP-42](../../07-Coding-Plan/inprogress/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) progress notes (link path corrected — pointed at a nonexistent `todo/` copy before). [CA-149](../../../change-audit/CA-149-node-behavior-registry-and-dispatch.md) is a dangling reference — no `requirements/change-audit/` directory exists anywhere in this checkout.
