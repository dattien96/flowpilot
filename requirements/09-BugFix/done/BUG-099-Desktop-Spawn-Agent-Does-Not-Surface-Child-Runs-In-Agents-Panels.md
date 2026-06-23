## Metadata

- Document ID: `BUG-099`
- Title: `Desktop Spawn Agent Does Not Surface Child Runs In Agents Panels`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-20`
- Last Updated: `2026-06-20`
- Parent Documents: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: none
- Related Documents: [BUG-087: Codex Resume Turns Bypass Approval MCP And AskUser](./BUG-087-Codex-Resume-Turns-Bypass-Approval-MCP-And-AskUser.md), [BUG-074: Desktop History Replay Leaves Resolved Approvals in Pending State](./BUG-074-Desktop-History-Replay-Leaves-Resolved-Approvals-Pending.md)
- Replaces: none
- Tags: desktop, agents, spawn-agent, orchestration, regression

## AI Quick View

### Summary

- A `spawn_agent` run could exist on the runner, but the desktop sidebar and main chat did not always receive the child graph updates needed to show it.
- The main bug was not the tool call name; it was the missing parent-level graph refresh when the child state changed.
- The fix emits the parent graph snapshot when the child run moves through its key lifecycle states.

### Current Ask

- Capture the missing-child-views regression as a standalone bugfix record.

### Key Decisions

- `V-1` Child run visibility must come from explicit graph updates, not from guessing based on the parent turn text.
- `V-2` The parent run should stay responsive while the child agent appears in the Agents UI.

### Constraints

- Do not change the `spawn_agent` tool contract.
- Do not hide the child run behind the parent prompt bubble.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/interactive_service_test.go`
- `apps/local-runner/internal/runner/agent_orchestrator_test.go`
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx`

## 1. Issue Summary

When the model called `spawn_agent`, the child run could be created in the runner without appearing promptly in the desktop agents sidebar or main orchestration view. The user would see the parent turn continue, but the child was effectively hidden.

## 2. Parent Links

- impacted coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot with the agent feature enabled
- reproduction steps:
  1. Ask the main AI to use `spawn_agent`.
  2. Wait for the child run to start or wait on a gate.
  3. Inspect the Agents sidebar and orchestration board.
- frequency: deterministic before the parent graph refresh fix

## 4. Expected vs Actual

- expected: child runs appear in the Agents UI as soon as the runner transitions them
- actual: the child could exist in the runner while the desktop view still looked like nothing happened

## 5. Impact

- users affected: users working with sub-agents
- workflows affected: spawn-agent delegation, child tracking, orchestration review
- severity: high, because the feature looks broken even when the runner created the child successfully

## 6. Root Cause

- hypothesis: the runner was not pushing the child graph state back to the parent UI consistently
- confirmed cause: the parent graph snapshot was not emitted on the key child state transitions that the desktop depends on
- evidence: the fix now emits parent graph updates when the child reaches `turn_started`, `permission_required`, `user_question_required`, `turn_completed`, and `turn_failed`

## 7. Fix Strategy

- `F-1` Emit parent graph updates from the child lifecycle transitions.
- `F-2` Keep the Agents sidebar and orchestration board reading from the same graph snapshot.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'TestSpawnChildRunWaitTrueWaitsThroughApprovalGate|TestSpawnChildRunEmitsParentGraphWhenChildWaitsApproval' -count=1` passes.
- `V-2` Desktop typecheck passes.

## 9. Regression Guard

- tests: runner tests covering child wait-state and parent graph emission
- alerts: none
- audit checks: child visibility must remain tied to explicit runner events

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: the `spawn_agent` tool contract stays the same; only the UI refresh path changed
