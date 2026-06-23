## Metadata

- Document ID: `BUG-104`
- Title: `Desktop Codex Child Spawn Failed On Unsupported Priority Tier`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-21`
- Last Updated: `2026-06-21`
- Parent Documents: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: none
- Related Documents: [BUG-099: Desktop Spawn Agent Does Not Surface Child Runs In Agents Panels](../done/BUG-099-Desktop-Spawn-Agent-Does-Not-Surface-Child-Runs-In-Agents-Panels.md), [BUG-100: YOLO Off Spawn Agent Child Approval Hangs Without Surfacing The Gate](../done/BUG-100-Yolo-Off-Spawn-Agent-Child-Approval-Hangs-Without-Surfacing-The-Gate.md)
- Replaces: none
- Tags: desktop, codex, spawn-agent, model-selection, priority, regression

## AI Quick View

### Summary

- SD-16 Test 4 failed before child approval because the spawned Codex child selected a model/configuration that requested the `priority` tier without supporting it.
- The user saw a child spawn failure instead of a waiting approval gate, so no child file write occurred.
- The fix makes child runs inherit the parent run's resolved model/reasoning defaults and makes Codex default to a full model before mini models.

### Current Ask

- Capture the unsupported-priority-tier child spawn regression as a standalone bugfix record.

### Key Decisions

- `V-1` Child runs should inherit the parent run's resolved model defaults unless an agent definition explicitly overrides the model.
- `V-2` Codex default chat selection should prefer a full enabled model before mini models when both are available.

### Constraints

- Do not change the `spawn_agent` contract.
- Do not convert YOLO-off approval into auto-approval.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/agent_orchestrator_test.go`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/state/store.test.ts`
- SD-16 Test 4 notes

## 1. Issue Summary

During SD-16 Test 4, the user asked the main AI to spawn a `coder` child under Codex with YOLO off. The child failed before the approval gate could appear, and the file was never written. The visible error said the requested `priority` tier was not supported for the child model.

## 2. Parent Links

- impacted coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot, Codex provider, YOLO off
- reproduction steps:
  1. Run SD-16 Test 4 exactly.
  2. Use `spawn_agent` with agent=`coder`, provider=`codex`, wait=`true`.
  3. Observe the child spawn fail before approval.
- frequency: reproducible with the unsupported child-model default

## 4. Expected vs Actual

- expected: child agent appears, enters `waiting_approval`, and resumes after approval
- actual: child spawn failed before the approval state could surface

## 5. Impact

- users affected: desktop users delegating work to Codex child agents
- workflows affected: spawn-agent delegation, child approval, file-writing approval flow
- severity: high, because the child does not start at all

## 6. Root Cause

- hypothesis: the child run inherited or selected a model/configuration that requested the `priority` service tier even when the chosen child model did not support it
- confirmed cause:
  - desktop Codex defaulting preferred `gpt-5.4-mini`, which is not the right fallback when a full Codex model is available
  - child spawn did not inherit the parent run's resolved model/reasoning defaults, so the child could fall onto a mismatched model/config combination
- evidence:
  - `models_cache.json` in the active Codex home shows `gpt-5.4-mini` without the `priority` service tier
  - the child spawn fix now inherits the parent model and reasoning settings, and desktop default selection prefers `gpt-5.5` before mini

## 7. Fix Strategy

- `F-1` Prefer a full enabled Codex model before mini models when selecting a default.
- `F-2` Copy the parent run's resolved model and reasoning defaults into the child start request unless the agent definition explicitly overrides the model.
- `F-3` Preserve the approval path so the child still waits for user approval when YOLO is off.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'SpawnChildRun|AgentOrchestrator|SpawnAgent' -count=1 -v` passed.
- `V-2` `npm --prefix apps/desktop-flowpilot run typecheck` passed.
- `V-3` `apps/desktop-flowpilot/node_modules/.bin/tsc -p tsconfig.phase1-tests.json` passed.

## 9. Regression Guard

- tests:
  - `TestSpawnChildRunInheritsParentModelDefaults`
  - `selectProvider defaults Codex to a full model before mini`
- alerts: none
- audit checks: child model defaults must never silently fall back to a mini model when a full enabled model exists

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: the spawn-agent contract and approval semantics stay unchanged
