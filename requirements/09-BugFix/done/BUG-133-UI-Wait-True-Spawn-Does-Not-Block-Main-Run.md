# BUG-133: UI Wait=True Spawn Does Not Block Main Run

## Metadata

- Document ID: `BUG-133`
- Title: `UI Wait=True Spawn Does Not Block Main Run`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [BUG-131: Spawn Agent Button Not Disabled While Main Busy](./BUG-131-Spawn-Agent-Button-Not-Disabled-While-Main-Busy.md), [CA-121: Agent panel runtime fixes](../../../change-audit/CA-121-agent-panel-runtime-fixes.md)
- Replaces: `None`
- Tags: `multi-agent, desktop, spawn, wait, ui`

## AI Quick View

### Summary

- BUG-131's gate keyed on the main run's turn status (`mainCardBusy`). A `wait=true` spawn from the desktop UI does not run a parent turn, so the main never looks "busy" — the user could still send prompts in the main chat and press `+ Spawn agent` while a wait=true child was running. SD-16 Test 5 failed.
- Fix: expose the spawn's `wait` flag on the agent summary (`waitForResult`) so the desktop can detect a running wait=true child and block both the chat send button and the spawn controls, regardless of the parent's turn status.

### Current Ask

- A running `wait=true` child must block the main run (no new prompt, no new spawn); `wait=false` children must allow concurrent spawns.

### Key Decisions

- `V-1` `AgentRunSummary` carries `waitForResult` (set from the spawn's wait flag, persisted on the run).
- `V-2` The desktop computes `hasBlockingChild = any running/ waiting child with waitForResult=true` and gates the chat send button and the spawn controls on it (in addition to `mainCardBusy`).
- `V-3` `wait=false` children never set the blocking flag, so concurrent background spawns remain allowed.

### Constraints

- Do not block on `wait=false` children.
- Do not change the backend spawn/blocking semantics; this adds a UI signal.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/agent_orchestrator.go` — `AgentRunSummary.WaitForResult`.
- `apps/local-runner/internal/runner/interactive_service.go` — summary construction at spawn, status updates, and `listAgentRunSummaries` set `WaitForResult`.
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx` — `hasBlockingChild` / `spawnBlocked`.
- `apps/desktop-flowpilot/src/components/ChatInput.tsx` — `hasBlockingChild` in `canSend` + placeholder.

## 1. Issue Summary

The desktop's busy gate used `mainCardBusy`, derived from the main run's turn status. A UI `wait=true` spawn blocks the HTTP spawn call but does not start a parent turn, so the main run stays idle and the gate never engaged: the user could send new prompts and spawn more agents while a wait=true child was still running.

## 2. Parent Links

- impacted coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md) (P-2, P-7)
- impacted tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md) (§7.2, Test 5)
- impacted system spec: [SS-06](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)

## 3. Environment and Reproduction

- environment: FlowPilot desktop chat.
- reproduction steps (SD-16 Test 5):
  1. Spawn an agent from the UI with Wait = ON.
  2. While it runs, type a prompt in the main chat — the send button is still enabled and sends.
  3. Press `+ Spawn agent` — still enabled, a second agent can be started.
- frequency: always for UI `wait=true` spawns.

## 4. Expected vs Actual

- expected: while a `wait=true` child runs, the main send button and spawn controls are disabled; `wait=false` children allow concurrent spawns.
- actual: nothing was blocked for UI `wait=true` spawns.

## 5. Impact

- users affected: anyone using `wait=true` UI spawns; breaks the documented wait contract and allows unbounded concurrent spawns.
- workflows affected: UI spawn, future auto agent management.
- severity: Medium-High — the wait semantics did not hold.

## 6. Root Cause

- confirmed cause: the gate signal (`mainCardBusy`) only reflected the main run's own turn status; a UI `wait=true` child runs without a parent turn, so the signal stayed false. The frontend had no signal identifying a running child as "blocking".

## 7. Fix Strategy

- `F-1` Add `WaitForResult` to the Go `AgentRunSummary` and set it from the run's wait flag at spawn, on status updates, and in `listAgentRunSummaries`.
- `F-2` Add `waitForResult` to the desktop `AgentRunSummary` contract.
- `F-3` In `AgentsPanel`, gate the spawn button and modal on `spawnBlocked = mainCardBusy || hasBlockingChild`.
- `F-4` In `ChatInput`, add `hasBlockingChild` to `canSend` and show a waiting placeholder.

## 8. Validation

- `V-1` `go test ./internal/runner/ -run TestAgentSummaryCarriesWaitForResultFlag -count=1` — pass (wait=false → false, wait=true → true).
- `V-2` No-regression: `go test ./internal/runner/ -run 'Yolo|Spawn|Agent|Turn|Approval' -count=1` with/without — identical failure set (5 pre-existing env failures); +1 new test. `go build ./internal/runner/...` — pass.
- `V-3` `npx tsc --noEmit` (desktop) — pass.
- `V-4` Manual (SD-16 Test 5): a running `wait=true` child disables main send + spawn; `wait=false` children allow concurrent spawns. The desktop vitest suite is pre-existing-broken (ESM config), so component tests could not run here.

## 9. Regression Guard

- tests: `TestAgentSummaryCarriesWaitForResultFlag`; SD-16 Test 5 documents the manual check.
- alerts: being able to send/spawn during a running `wait=true` child indicates the flag stopped propagating.
- audit checks: every live `AgentRunSummary` construction must set `WaitForResult` from the run.

## 10. Follow-Up Document Updates

- upstream docs that must change: SD-16 Test 5 annotated as the regression guard for this fix.
- notes left unchanged on purpose: backend wait/blocking semantics unchanged; this adds a UI signal only.
