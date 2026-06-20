## Metadata

- Document ID: `BUG-100`
- Title: `YOLO Off Spawn Agent Child Approval Hangs Without Surfacing The Gate`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-20`
- Last Updated: `2026-06-20`
- Parent Documents: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md), [SD-09: Approval Gates & YOLO Mode](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- Child Documents: none
- Related Documents: [BUG-087: Codex Resume Turns Bypass Approval MCP And AskUser](./BUG-087-Codex-Resume-Turns-Bypass-Approval-MCP-And-AskUser.md), [BUG-071: Codex YOLO-Off Workspace Write And Selected Skill Names Regressed](./BUG-071-Codex-Yolo-Off-Workspace-Write-And-Selected-Skill-Names-Regressed.md), [BUG-072: Codex MCP Elicitation Approval Unsupported](./BUG-072-Codex-MCP-Elicitation-Approval-Unsupported.md)
- Replaces: none
- Tags: desktop, codex, spawn-agent, approval, yolo, regression

## AI Quick View

### Summary

- In a YOLO-off spawn-agent scenario, the child could reach an approval gate while the parent turn looked stuck and the child gate was not surfaced clearly in the desktop.
- The user observed the child write happen, which means the approval bridge and visibility path were both wrong from a UX perspective.
- The fix keeps approval state visible on the child run and prevents the parent UI from looking frozen.

### Current Ask

- Capture the YOLO-off child-approval hang as a standalone bugfix record.

### Key Decisions

- `V-1` Child approval state must be shown in the child run and in the parent orchestration UI.
- `V-2` The parent run should not look frozen while waiting on child approval.

### Constraints

- Do not convert YOLO off into auto-approval.
- Do not hide approval cards when the child is waiting.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/codex_adapter.go`
- `apps/local-runner/internal/runner/codex_appserver.go`
- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/codex_appserver_test.go`
- `apps/local-runner/internal/runner/interactive_service_test.go`

## 1. Issue Summary

When the user spawned a child agent with `wait=true` and YOLO was off, the child could hit a write approval gate, the file write still happened, and the main chat looked hung instead of surfacing a clean waiting state. The user expected the child to pause visibly until approval.

## 2. Parent Links

- impacted coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot, Codex provider, YOLO off
- reproduction steps:
  1. Ask the main AI to call `spawn_agent` once with `wait=true`.
  2. Use a child prompt that triggers a file write.
  3. Observe the parent turn and child UI state.
- frequency: deterministic before the approval-bridge and graph-surface fixes

## 4. Expected vs Actual

- expected: the child run shows a visible approval gate, the parent stays responsive, and the child resumes only after approval
- actual: the parent looked hung and the child gate was not surfaced clearly enough

## 5. Impact

- users affected: users delegating work to child agents with approval gates enabled
- workflows affected: child file writes, child command execution, approval flows
- severity: high, because it blocks trust in the child-agent workflow

## 6. Root Cause

- hypothesis: the resumed Codex child path was not surfacing approval/question events through the same bridge as the fresh path
- confirmed cause: the child flow did not keep the approval state visible in the desktop graph/UI while the parent waited for completion
- evidence: the fix reuses the app-server bridge path and the parent graph emission path so approval state is shown instead of appearing frozen

## 7. Fix Strategy

- `F-1` Keep child approval and question events on the bridge-capable path.
- `F-2` Surface the waiting child state in the parent orchestration UI.
- `F-3` Preserve YOLO-off manual approval semantics.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'TestCodexAdapterResumedTurnRoutesAskUserDynamicTool|TestSpawnChildRunWaitTrueWaitsThroughApprovalGate|TestSpawnChildRunEmitsParentGraphWhenChildWaitsApproval' -count=1` passes.
- `V-2` Desktop typecheck passes.

## 9. Regression Guard

- tests: runner approval bridge tests and parent graph emission tests
- alerts: none
- audit checks: YOLO off must still require manual approval

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: the approval policy stays strict; only the runtime/UI surfacing changed
