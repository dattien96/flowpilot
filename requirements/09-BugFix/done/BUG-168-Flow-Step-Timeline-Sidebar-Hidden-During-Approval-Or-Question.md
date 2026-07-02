# BUG-168: Flow Step Timeline Sidebar Hidden During Approval Or Question

## Metadata

- Document ID: `BUG-168`
- Title: `Flow Step Timeline Sidebar Hidden During Approval Or Question`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-156-Flow-Timeline-Moved-To-Dedicated-Collapsible-Sidebar.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-158-Flow-Timeline-Missing-Step-Name-Model-Agent-Flow-Yolo.md`, `requirements/09-BugFix/done/BUG-159-Flow-Sidebar-Progress-Count-And-Yolo-Visibility.md`
- Replaces: `none`
- Tags: `agent-flow-engine, ui, desktop, workflow-steps-runtime, flow-mode, regression`

## AI Quick View

### Summary

- In Flow Mode, the dedicated step-timeline sidebar (`FlowTimelineSidebar`) disappears the instant an approval form or a `mcp__flowpilot__ask_user` question form is shown, then reappears once the turn resumes.
- Root cause is a too-narrow visibility predicate: the sidebar renders only while `runStatus === "running"`, but a pending approval/question flips the run status to `waiting_approval` / `waiting_question`, so the whole `<aside>` unmounts.
- The step timeline is exactly the context a user wants *while* deciding on an approval, so hiding it there is the worst possible moment.

### Current Ask

- Keep the Flow Mode step-timeline sidebar mounted and visible while the run is paused on an approval or a question, not only while it is actively streaming.

### Key Decisions

- `V-1` The sidebar is "flow-active" for `running`, `waiting_approval`, and `waiting_question` — all non-terminal, in-flight states — and only hides on terminal states (`completed` / `failed` / `cancelled`) or when not in Flow Mode.

### Constraints

- Pure display-logic change in one component; no change to run-status semantics or the step-runtime data path.
- Must not make the sidebar appear for `normal_chat` runs (the `isFlowModeRun(chatMode)` guard stays).

### Open Questions

- Should the sidebar also remain during `starting`? Current behavior never showed it then (no step data yet); leaving `starting` out preserves that.

### Source Refs

- `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx:20` (`visible` predicate)
- `apps/desktop-flowpilot/src/state/store.ts:1126` (status → `waiting_approval`)
- `apps/desktop-flowpilot/src/state/store.ts:1142` (status → `waiting_question`)
- `apps/desktop-flowpilot/src/types/contract.ts:277` (`RunStatus` union)

## 1. Issue Summary

While a Flow Mode run is executing and an approval card (e.g. a `Bash` command) or an `ask_user` question card appears, the middle step-timeline sidebar vanishes. When the user approves/answers and the turn continues, the sidebar comes back. The timeline should stay visible throughout — the user needs it most while weighing an approval.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, a live Flow Mode (`workflow_step_auto`) run against a runner.
- reproduction steps:
  1. Launch a Flow Mode run (Workflow / Review Loop) so the step-timeline sidebar is showing.
  2. Let the run reach a tool call that needs approval (or an `mcp__flowpilot__ask_user` question).
  3. Observe the step-timeline sidebar disappears while the approval/question card is pending.
  4. Approve/answer; the sidebar reappears when the run returns to `running`.
- frequency: deterministic — every approval/question in Flow Mode.

## 4. Expected vs Actual

- expected: the step-timeline sidebar stays visible (and keeps showing the current step / progress) while the run is paused on an approval or question.
- actual: the sidebar unmounts entirely for the duration of the pending approval/question.

## 5. Impact

- users affected: anyone running Flow Mode that involves approvals or `ask_user` questions.
- workflows affected: Flow Mode UI only; no data loss.
- severity: medium — not a data bug, but it removes key orchestration context at the exact moment the user is making a decision, and reads as a flicker/instability.

## 6. Root Cause

- hypothesis: the sidebar visibility is gated on a single status value.
- confirmed cause: `FlowTimelineSidebar` computes `const visible = isFlowModeRun(chatMode) && runStatus === "running";` (`FlowTimelineSidebar.tsx:20`) and returns `null` when `!visible` (`:27`). When an approval is queued the store sets `status: "waiting_approval"` (`store.ts:1126`) and for questions `status: "waiting_question"` (`store.ts:1142`). Neither equals `"running"`, so `visible` is `false` and the component unmounts.
- evidence: `RunStatus` includes `waiting_approval` / `waiting_question` as distinct non-terminal states (`contract.ts:277-285`); the store transitions into them while a run is still in flight.

## 7. Fix Strategy

- `F-1` Broadened the predicate in `FlowTimelineSidebar` to treat `running`, `waiting_approval`, and `waiting_question` as visible: `const visible = isFlowModeRun(chatMode) && (runStatus === "running" || runStatus === "waiting_approval" || runStatus === "waiting_question");`. Implemented at `FlowTimelineSidebar.tsx:20-25`.

## 8. Validation

- `V-1` `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-2` Not executed: a live Flow Mode run through an approval/question pause, since no runner + provider account is available in this environment. The predicate change is small and directly mirrors the `RunStatus` union already exercised by `AgentsPanel`'s equivalent `mainCardBusy` check (`AgentsPanel.tsx:100`), which reduces risk, but the user should confirm the sidebar stays visible on next live use.

## 9. Regression Guard

- tests: consider a small unit test around the visibility predicate if it is extracted into a testable helper; otherwise none (no render-test harness for this component, consistent with BUG-156/158/159).
- alerts: none.
- audit checks: to be recorded in a `change-audit/CA-xxx-*` note when the fix lands.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: `starting` deliberately not added to the visible set (no step data exists yet at that point).
