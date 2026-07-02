# BUG-153: Flow Mode Right Sidebar Missing Step Execution State

## Metadata

- Document ID: `BUG-153`
- Title: `Flow mode right sidebar does not surface workflow step execution state`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `CP-41, CP-42`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md)
- Child Documents: [Task-181: Flow Mode Workflow Step Runtime Sidebar](../../08-Task/done/Task-181-Flow-Mode-Workflow-Step-Runtime-Sidebar.md)
- Related Documents: [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/done/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [Task-177: Review Loop Pack And Chat Picker](../../08-Task/todo/Task-177-Review-Loop-Pack-And-Chat-Picker.md)
- Replaces: `None`
- Tags: `flow-mode, desktop, right-sidebar, workflow-step, runtime-state, ui, orchestration`

## AI Quick View

### Summary

- In Flow mode, the Go runner already knows the runtime step list and status progression (`PENDING`, `RUNNING`, `WAITING_USER_APPROVAL`, `DONE`, `FAILED`, `SKIPPED`) through `RuntimeWorkflowStep` and the shared workflow state machine.
- The desktop chat shell, however, only renders generic right-rail panels (`WorkflowControlPanel`, `ChatStartIntentPanel`, `AgentsPanel`, `ProviderAccountsPanel`) and does not project Flow mode step-runtime state into a dedicated sidebar view.
- As a result, users running a known flow cannot see, at a glance, which step is active, which step finished, whether a turn is the first attempt or a retry, or whether Flow mode is currently blocked on validation / approval / retry.
- This is especially confusing for CP-41/CP-42 flows because the system already has deterministic step progression, validation retry, and audit-draft events, but the UI still feels like normal chat.

### Current Ask

- Add a Flow-mode-only right sidebar panel that shows the current workflow step list and runtime state, including active step, completed steps, failed/retrying steps, and whether the current execution is a fresh pass or retry.

### Key Decisions

- `V-1` Flow mode continues to use the same chat workspace shell as normal chat, but the right sidebar becomes mode-aware and renders a dedicated step-runtime panel when the active run is workflow/step-driven rather than `normal_chat`.
- `V-2` The source of truth for step status remains Go runner workflow state, not AI inference and not timeline text parsing.
- `V-3` Chat mode Review Loop and agent-flow board semantics remain unchanged; this fix is specifically about projecting linear workflow-step runtime to the sidebar for Flow mode.
- `V-4` Retry state must be explicit. A user should be able to distinguish "currently running first attempt" from "running after validation failure / reprompt / retry".

### Constraints

- Keep the existing shared chat shell. Do not fork a separate Flow mode page.
- Reuse generic workflow runtime data where possible; do not hardcode review-loop or RAG-harness-specific step names into desktop logic.
- The new UI must degrade gracefully for normal chat and for older runs that do not yet persist the richer step-runtime projection.
- Because GitNexus MCP tools are not available in this Codex session, implementation must run local code inspection only unless those tools are restored later.

### Open Questions

- `Q-1` Should the desktop poll `GET /client/workflow-runs/{runId}` plus a new step-runtime endpoint, or should the event stream carry a dedicated `workflow_progress_snapshot` event so the sidebar is fully event-driven?
- `Q-2` Should retry reason be represented as a normalized enum (`validation_retry`, `gate_retry`, `manual_resume`) or as a display-only label derived from existing events?
- `Q-3` For agent-flow-as-one-step scenarios, should the panel show only the outer workflow steps, or allow drilling into the embedded agent graph from the same card?

### Source Refs

- `apps/local-runner/internal/runner/workflow_state_machine.go`
- `apps/local-runner/internal/runner/interactive_handlers.go`
- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/runner/provider_event.go`
- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`
- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/types/contract.ts`

## 1. Issue Summary

Flow mode already runs on deterministic workflow steps, but the desktop UI does not expose that determinism in the right sidebar. Users can see the chat transcript and, in some cases, the agent graph, yet cannot clearly answer simple operational questions such as:

- which step is running right now
- which steps are already done
- whether the current Coding step is the first pass or a retry
- whether the flow is blocked on approval, validation, or a failed step

This creates a mismatch between backend capability and frontend visibility. The system behaves like a workflow engine, but the UI still presents it like a generic chat.

## 2. Parent Links

- impacted coding plan: [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md)
- impacted tech design: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-05: Workflow Engine](../../06-System-Tech-Design/SD-05-Workflow-Engine.md)
- impacted system spec: [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md), [SS-04: Workflow](../../05-System-Specs/SS-04-Workflow.md)

## 3. Environment and Reproduction

- environment: Desktop app, local runner, Flow mode launch (`chatMode = "workflow_step_auto"`), any workflow with multiple runtime steps such as CP-41 Plan/Coding/Testing/Audit.
- reproduction steps:
  1. Open desktop app and switch to Flow mode.
  2. Launch a workflow or step-backed run.
  3. Observe the right sidebar while the run moves through planning/coding/testing/audit.
  4. Trigger a retry path if available, for example validation failure leading back to Coding.
  5. Compare what the runner knows versus what the user can see in the sidebar.
- frequency: Always reproducible for Flow mode runs.

## 4. Expected vs Actual

- expected: The right sidebar shows a step list with per-step runtime state, active marker, terminal states, and retry hints so the user can track Flow mode as a workflow.
- actual: The right sidebar shows only generic panels. Flow mode runtime step progression is largely invisible unless the user reconstructs it manually from chat text and agent events.

## 5. Impact

- users affected: Anyone running Flow mode workflows, especially debugging-heavy or iterative flows.
- workflows affected: CP-41 harness flows, linear workflow-step runs, and any future Flow mode orchestration that depends on deterministic step progression.
- severity: Medium-high. This is a UX/state-observability gap rather than data loss, but it makes Flow mode hard to trust and hard to debug.

## 6. Root Cause

- hypothesis: The desktop shell was intentionally built as a shared chat UI, but no mode-aware projection layer was added for workflow-step runtime.
- confirmed cause:
  - Go already models runtime steps via `RuntimeWorkflowStep` and progresses them through `PlanWorkflowProgress`.
  - The interactive HTTP contract currently exposes run snapshot, event stream, and agent graph, but no dedicated workflow-step runtime DTO is present in `contract.ts`.
  - `ChatWorkspace.tsx` right rail is composed of generic panels only and has no `WorkflowStepRuntimePanel`.
  - `store.ts` tracks run status, agent graph, approvals, questions, gate blocks, and artifacts, but does not keep a step-runtime collection for the active Flow mode run.
- evidence:
  - `workflow_state_machine.go` defines canonical statuses and transitions.
  - `interactive_handlers.go` exposes `GET /client/workflow-runs/{runId}` and agent graph APIs, but `runSnapshotView` contains no workflow-step list.
  - `provider_event.go` already has flow-specific events like `flow_context_package`, `flow_validation_result`, `flow_validation_retry`, and `flow_audit_draft`, but the TS client contract currently only models up to `flow_gate_violation` and agent graph events.
  - `ChatWorkspace.tsx` right sidebar stack contains no workflow-step runtime panel.

## 7. Fix Strategy

### 7.1 Backend Contract

- `F-1` Introduce a client-facing workflow step runtime DTO, for example:
  - `WorkflowStepRuntimeDTO { stepId, stepType, label, status, retryCount, rejectionNote, startedAt, finishedAt, requiresApproval, behaviorId }`
- `F-2` Extend the run snapshot API or add a dedicated endpoint such as `GET /client/workflow-runs/{runId}/steps-runtime` to expose the ordered runtime step list from `workflowStore.LoadRunSteps`.
- `F-3` Optionally add an event-stream projection for step progress changes so the desktop can update the sidebar incrementally without manual refresh after every transition.

### 7.2 Runner Projection Rules

- `F-4` Keep Go workflow state as the single source of truth. Do not derive step status from AI prose or child agent messages.
- `F-5` Map existing step runtime and flow-specific events into richer UI semantics:
  - `RUNNING` + `retryCount == 0` => fresh execution
  - `RUNNING` + `retryCount > 0` => retry execution
  - `WAITING_USER_APPROVAL` => blocked on approval
  - `FAILED` => terminal failure
  - `DONE` => completed
- `F-6` For CP-41 retry flows, reuse existing validation retry logic rather than inventing a new retry detector in frontend. If needed, persist a last retry reason in Go so the UI can say "retry after validation failure".

### 7.3 Desktop Store

- `F-7` Extend `apps/desktop-flowpilot/src/types/contract.ts` with the workflow step runtime DTO and any new event type if added.
- `F-8` Extend `store.ts` with:
  - `workflowStepRuntime?: WorkflowStepRuntimeDTO[]`
  - `workflowStepRuntimeLoading: boolean`
  - `refreshWorkflowStepRuntime(): Promise<void>`
  - selectors/helpers for `activeStep`, `hasRetries`, `isFlowModeRun`
- `F-9` On Flow mode start/resume/open-history, load the step-runtime projection alongside run snapshot and keep it refreshed after relevant events:
  - `turn_started`
  - `turn_completed`
  - `turn_failed`
  - `flow_validation_result`
  - `flow_validation_retry`
  - `flow_audit_draft`
  - approval/question settle transitions

### 7.4 Right Sidebar UI

- `F-10` Add a new `WorkflowStepRuntimePanel` to the right sidebar stack in `ChatWorkspace.tsx`.
- `F-11` Render this panel only when the current run is Flow mode or workflow/step-backed history, not for `normal_chat`.
- `F-12` Panel content should include:
  - ordered step list
  - per-step icon/color for `PENDING/RUNNING/WAITING_USER_APPROVAL/DONE/FAILED/SKIPPED`
  - retry badge such as `Retry 1`, `Retry 2`
  - current active step marker
  - optional small detail row from `rejectionNote` or normalized retry reason
- `F-13` Keep `AgentsPanel` separate. Outer workflow-step progress and inner agent graph are different layers and should not be merged into one ambiguous card.

### 7.5 Flow-Mode UX Semantics

- `F-14` Make the panel explicitly Flow-mode-aware:
  - Flow mode run: show workflow step runtime first
  - Normal chat with Review Loop selected: keep existing chat/agent feel, no linear workflow-step list unless the run is actually workflow-backed
- `F-15` If a flow embeds agent flow as one workflow step, represent the outer step as one row and let the user inspect the agent graph via the existing board/panel rather than flattening agent children into fake workflow steps.

## 8. Detailed Coding Plan

### 8.1 Phase A — Contract and Runner Read Path

- add TS/Go DTOs for step-runtime projection
- implement read path from `workflowStore.LoadRunSteps`
- expose ordered runtime steps over interactive API
- add tests for empty chat run, workflow run, and resumed workflow run

### 8.2 Phase B — Desktop State Wiring

- load step-runtime data when a workflow run becomes active
- cache it per run in the existing run snapshot map if needed
- refresh it after flow-state-changing events
- guard normal chat so it does not make unnecessary requests

### 8.3 Phase C — Sidebar UI

- create `WorkflowStepRuntimePanel.tsx`
- insert it above `AgentsPanel` in the right rail for Flow mode
- style status chips/icons using existing theme variables
- ensure compact but scannable rendering for 4-10 steps

### 8.4 Phase D — Retry and Failure Semantics

- define exact user-facing labels for retry states
- decide whether `rejectionNote` alone is sufficient or a normalized retry reason field is needed
- ensure failed step + resumed step does not duplicate rows; one step row should evolve over time

### 8.5 Phase E — History and Resume

- when reopening an old Flow mode run from history, restore step-runtime state
- if older sessions lack enough projection data, fetch from runner store and degrade gracefully when absent
- verify that resumed runs keep the correct active step indicator

## 9. Validation

- `V-1` Start a CP-41-style Flow mode run and verify Plan -> Coding -> Testing -> Audit rows progress correctly.
- `V-2` Trigger validation retry and verify the Coding row shows retry state rather than a brand-new duplicated step row.
- `V-3` Trigger a waiting-approval step and verify the sidebar shows blocked/approval state.
- `V-4` Reopen a persisted Flow mode run from history and verify the sidebar reconstructs the correct current step state.
- `V-5` Open a normal chat run and verify the new panel is hidden and no behavior regresses.
- `V-6` Existing agent graph / Review Loop controls still work unchanged.

## 10. Regression Guard

- tests:
  - Go handler test for workflow step runtime endpoint/projection
  - desktop store test for refresh after flow events
  - component test for `WorkflowStepRuntimePanel`
  - history resume test for workflow-backed runs
- alerts: none
- audit checks:
  - confirm no workflow-specific step names are hardcoded in desktop rendering
  - confirm normal chat runs do not request or render workflow-step runtime data

## 11. Follow-Up Document Updates

- upstream docs that must change:
  - CP-41 should mention the desktop Flow mode observability panel once implemented.
  - CP-42 should note that generic behaviors can drive Flow mode UI only after Go projects runtime step state into the desktop contract.
- notes left unchanged on purpose:
  - No change to flow execution semantics is intended.
  - No change to Review Loop pack behavior is required for this bugfix; only Flow mode visibility is being improved.
- resolution: Implemented in [Task-181](../../08-Task/done/Task-181-Flow-Mode-Workflow-Step-Runtime-Sidebar.md) — `GET /client/workflow-runs/{runId}/steps-runtime` projects `workflowStore.LoadRunSteps` into a client DTO; the desktop store loads/refreshes it at turn and history-open boundaries; a new Flow-mode-only `WorkflowStepRuntimePanel` renders it above `AgentsPanel`. `go build`/`go test ./internal/runner/...` and `tsc --noEmit` pass (new `TestWorkflowStepsRuntime*` Go tests included). Not verified: live Electron/browser rendering of the panel — `desktop-flowpilot`'s own bootstrap gate requires a running local-runner + Supabase + Electron preload bridge, out of reach in this session; V-1..V-6 above remain a manual follow-up.
