# Task-181: Flow Mode Workflow Step Runtime Sidebar

## Metadata

- Document ID: `Task-181`
- Title: `Add Flow-mode-only right sidebar panel projecting Go workflow-step runtime state`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `CP-41, CP-42`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md)
- Child Documents: `None`
- Related Documents: [BUG-153: Flow Mode Right Sidebar Missing Step Execution State](../../09-BugFix/done/BUG-153-Flow-Mode-Right-Sidebar-Missing-Step-Execution-State.md), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/done/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md)
- Replaces: `None`
- Tags: `flow-mode, desktop, right-sidebar, workflow-step, runtime-state, ui, orchestration`

## AI Quick View

### Summary

- Go already tracks per-step runtime state (`RuntimeWorkflowStep`, `workflowStore.LoadRunSteps`) but the desktop right sidebar had no view of it, so Flow mode runs looked like plain chat even though they are deterministic workflow-step runs.
- This task adds a read-only client-facing projection: a new Go HTTP endpoint, a matching TS DTO, store wiring, and a new `WorkflowStepRuntimePanel` shown only for Flow-mode/workflow-step-backed runs, above `AgentsPanel`.
- Go's `RuntimeWorkflowStep` remains the single source of truth; no step status is inferred from chat text or AI output.

### Current Ask

- Execute BUG-153's fix strategy (7.1-7.5) and coding plan (8.1-8.5) end to end: backend read path, desktop contract/store wiring, and the new sidebar panel with retry/approval/failure semantics and history-resume support.

### Key Decisions

- `T-1` Added `GET /client/workflow-runs/{runId}/steps-runtime`, backed by the existing `workflowStore.LoadRunSteps`, returning a new `workflowStepsRuntimeSnapshot` DTO. 404s the same way `GET /client/workflow-runs/{runId}` does for an unknown run, so normal chat / stale history degrades identically.
- `T-2` Added `FinishedAt` to `RuntimeWorkflowStep` (previously only present on the mutation-side `WorkflowStepPatch`) so the DTO can report step completion time; wired through `SupabaseWorkflowStore.LoadRunSteps` (added `finished_at` to the PostgREST select) and the in-memory `fakeWorkflowStore.ApplyStepTransition`. This is additive only — no existing caller uses positional struct literals, confirmed by inspection.
- `T-3` `rejectionNote` on the DTO doubles as the display-only retry reason (resolves BUG-153 Q-2) instead of inventing a new normalized enum — reuses the existing validation-retry data path (F-6) rather than adding a new classifier.
- `T-4` The panel is Flow-mode-only (`chatMode === "workflow_step_auto"`), inserted above `AgentsPanel` in `ChatWorkspace.tsx`'s right rail; Review Loop / normal chat keep their existing look (F-14).
- `T-5` `refreshWorkflowStepRuntime()` mirrors the existing `refreshAgentRuns()` pattern (stale-response sequence guard) and is called from the same three call sites: after a turn completes (`sendPrompt`), on child-run focus, and on `openHistoryRun` — so a reopened persisted Flow-mode run restores step state (resolves BUG-153 V-4 / Phase E).
- `T-6` Derived helpers (`isFlowModeRun`, `activeWorkflowStep`, `hasRetries`) are plain exported functions over `workflowStepRuntime` + `chatMode`, not extra stored state, per BUG-153 F-8's intent without duplicating source of truth.

### Constraints

- Did not fork a separate Flow mode page; the shared `ChatWorkspace.tsx` shell is unchanged for normal chat.
- No workflow/review-loop-specific step names are hardcoded in the new desktop code; the panel renders whatever `stepType` the runner reports.
- GitNexus MCP tools were unavailable in this session (consistent with BUG-153's own note); implementation proceeded via local code inspection only, per the `add-new-task` skill's documented fallback.

### Open Questions

- `Q-1` (BUG-153) resolved as: poll-based (`GET .../steps-runtime`) refreshed at turn/history boundaries, not a dedicated SSE event type — kept in scope, no new event plumbing added.
- `Q-2` (BUG-153) resolved as: display-only label from `rejectionNote`, no new enum (see T-3).
- `Q-3` (BUG-153) intentionally deferred: the panel shows outer workflow steps only; drilling into an embedded agent graph from the same row is out of scope for this task (see Out of Scope).

### Source Refs

- `CP-41`, `CP-42`, `BUG-153`, `SS-16`, `SD-19`

## 1. Goal

Give the desktop Flow-mode right sidebar a live, Go-sourced view of workflow step runtime state: which step is active, which are done, and whether a step is on a retry or blocked pass — without forking the chat shell or inferring state from chat text.

## 2. Parent Links

- coding plan: [CP-41](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md), [CP-42](../../07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md)
- tech design: [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- system spec: [SS-16](../../05-System-Specs/SS-16-Agent-Flow-Engine.md)
- specific upstream ids: `BUG-153`

## 3. Trigger

BUG-153 reported that Flow mode users cannot see step-by-step runtime state (active step, done steps, retry/approval/failure) even though the Go runner already computes it deterministically via the workflow state machine.

## 4. Exact Change

- `T-1` `apps/local-runner/internal/runner/workflow_state_machine.go`: add `FinishedAt string` to `RuntimeWorkflowStep`.
- `T-2` `apps/local-runner/internal/runner/supabase_workflow_store.go`: select `finished_at`, map it into `RuntimeWorkflowStep`.
- `T-3` `apps/local-runner/internal/runner/workflow_store.go`: apply `FinishedAt` patches in `fakeWorkflowStore.ApplyStepTransition`.
- `T-4` `apps/local-runner/internal/runner/interactive_handlers.go`: add `workflowStepRuntimeView` / `workflowStepsRuntimeSnapshot` DTOs, `workflowStepsRuntime()` snapshot builder, `handleGetWorkflowStepsRuntime`, and register `GET /client/workflow-runs/{runId}/steps-runtime`.
- `T-5` `apps/local-runner/internal/runner/workflow_step_runtime_test.go` (new): handler tests for a fresh workflow run's ordered steps, a retry transition evolving the same row in place, and a 404 for an unknown run.
- `T-6` `apps/desktop-flowpilot/src/types/contract.ts`: add `WorkflowStepRuntimeStatus`, `WorkflowStepRuntimeDTO`, `WorkflowStepsRuntimeSnapshot`, and an optional `getWorkflowStepsRuntime` method on `RunnerClient`.
- `T-7` `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`: implement `getWorkflowStepsRuntime`.
- `T-8` `apps/desktop-flowpilot/src/state/store.ts`: add `workflowStepRuntime`, `workflowStepRuntimeLoading`, `_workflowStepRuntimeLoadSeq` state; `refreshWorkflowStepRuntime()` action (same stale-response-guard shape as `refreshAgentRuns`); call it at the same 3 sites `refreshAgentRuns()` is called (post-turn, agent focus, history open); clear `workflowStepRuntime` alongside `agentRuns` on run reset/handoff; add `isFlowModeRun`, `activeWorkflowStep`, `hasRetries` selector helpers.
- `T-9` `apps/desktop-flowpilot/src/components/WorkflowStepRuntimePanel.tsx` (new): Flow-mode-only panel rendering the ordered step list with status dot, label, retry badge, and rejection note.
- `T-10` `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`: render `<WorkflowStepRuntimePanel />` above `<AgentsPanel />`.
- `T-11` `apps/desktop-flowpilot/src/styles.css`: add `.sd.fail`, `.wsr-empty`, `.wsr-retry-badge` (reuses existing `.panel`/`.acard`/`.sd` classes for everything else).

## 5. Touched Areas

- files: see T-1..T-11 above.
- modules: local-runner `runner` package (Go), desktop-flowpilot `state`/`components`/`client`/`types`.
- routes: `GET /client/workflow-runs/{runId}/steps-runtime` (new).
- tables: none changed; reads existing `workflow_run_steps` (adds `finished_at` to an existing select).

## 6. Acceptance Check

- `go build ./...` and `go test ./internal/runner/...` pass in `apps/local-runner` — new `TestWorkflowStepsRuntime*` tests (ordered steps, retry-in-place, 404) pass. Pre-existing failures (Codex CLI unavailable, Windows-path-specific expectations) are unrelated to this change and reproduce identically on a clean checkout.
- `npx tsc --noEmit` passes in `apps/desktop-flowpilot` after all contract/store/component changes.
- Not verified: live browser/Electron rendering of the new panel. `preview_start` on `desktop-flowpilot` only reaches the app's own "Bootstrapping desktop workspace…" gate, which requires a running local-runner process + Supabase config + the Electron preload bridge — infrastructure outside this task's scope to stand up. This is a pre-existing constraint of previewing this Electron app, not something introduced by this change.

## 7. Out of Scope

- BUG-153 Q-3 (drilling into an embedded agent graph from a workflow-step row) — outer steps only, per F-15.
- Any new SSE/event-stream projection for step progress (BUG-153 F-3) — this task uses the existing turn/history refresh points instead.
- Introducing a JS/TS test runner for `desktop-flowpilot` (none exists in the repo today) — a proper component test for `WorkflowStepRuntimePanel` is a separate follow-up once test infra exists.
- Any change to Review Loop pack behavior, agent-flow board semantics, or flow execution semantics.

## 8. Completion Notes

- result: Implemented and type/build-verified per section 6. BUG-153 marked resolved and moved to `done/`.
- follow-ups:
  - Add a desktop component/unit test for `WorkflowStepRuntimePanel` once a JS test runner is introduced for `apps/desktop-flowpilot`.
  - Manually verify the panel in a running Electron + local-runner + Supabase environment (BUG-153 V-1..V-6) — not exercisable from this session.
  - Consider BUG-153 Q-3 (agent-graph drill-in) as a follow-up task if users ask for it.
- upstream docs updated: `BUG-153` moved to `done/` with status updated and this task linked back.
