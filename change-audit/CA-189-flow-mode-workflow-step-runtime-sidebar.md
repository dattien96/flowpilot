# CA-189: Flow Mode Right Sidebar Surfaces Workflow Step Runtime (BUG-153)

## Scope

Implemented `BUG-153` / `Task-181`: the desktop Flow-mode right sidebar had no view of the Go runner's authoritative workflow-step runtime state (`RuntimeWorkflowStep`), so a deterministic Flow-mode run looked like plain chat — no active-step indicator, no retry/approval/failure visibility.

## The bug

Go's workflow state machine already tracked per-step status (`PENDING/RUNNING/WAITING_USER_APPROVAL/DONE/FAILED/SKIPPED`), retry count, and rejection notes via `workflowStore.LoadRunSteps`, but nothing in the interactive HTTP contract exposed that list to the desktop, and `ChatWorkspace.tsx`'s right rail only rendered generic panels (`WorkflowControlPanel`, `ChatStartIntentPanel`, `AgentsPanel`, `ProviderAccountsPanel`).

## Fix

- Backend: added `GET /client/workflow-runs/{runId}/steps-runtime`, backed directly by `workflowStore.LoadRunSteps` (no AI-inferred or timeline-derived state). Added `FinishedAt` to `RuntimeWorkflowStep` (previously only on the mutation-side `WorkflowStepPatch`) and threaded it through `SupabaseWorkflowStore.LoadRunSteps` and `fakeWorkflowStore.ApplyStepTransition` so the new DTO can report step completion time.
- Desktop contract: added `WorkflowStepRuntimeDTO`/`WorkflowStepsRuntimeSnapshot` types and an optional `getWorkflowStepsRuntime` method on `RunnerClient`, implemented in `HttpWsRunnerClient`.
- Desktop store: added `workflowStepRuntime` state + `refreshWorkflowStepRuntime()` (same stale-response-guard shape as the existing `refreshAgentRuns()`), called from the same three sites `refreshAgentRuns()` already uses (post-turn, agent focus, history-open) so a reopened persisted Flow-mode run restores step state. Guarded to no-op outside `chatMode === "workflow_step_auto"` so normal chat never requests it.
- Desktop UI: new `WorkflowStepRuntimePanel.tsx`, Flow-mode-only, rendered above `AgentsPanel` in `ChatWorkspace.tsx`'s right rail. Reuses existing `.panel`/`.acard`/`.sd` status-dot classes; adds only `.sd.fail`, `.wsr-empty`, `.wsr-retry-badge` to `styles.css`. `rejectionNote` doubles as the display-only retry reason rather than a new normalized enum.

## Verification

- `go build ./...` and `go test ./internal/runner/...` pass. New `workflow_step_runtime_test.go` covers: ordered steps for a fresh workflow run, a retry transition evolving the same step row in place (no duplicate row) with `retryCount`/`rejectionNote` reflected, and a 404 for an unknown run. Pre-existing failures (Codex CLI unavailable, Windows-path-specific expectations) are unchanged and unrelated.
- `npx tsc --noEmit` passes in `apps/desktop-flowpilot` after all contract/store/component changes.
- Not verified: live Electron/browser rendering of the panel. `desktop-flowpilot`'s own bootstrap gate ("Bootstrapping desktop workspace…") requires a running local-runner process, Supabase config, and the Electron preload bridge — none of which were available to stand up in this session. This is a pre-existing constraint of previewing this Electron app, not introduced by this change, and is recorded as a follow-up in `Task-181`.
- No JS/TS test runner exists for `apps/desktop-flowpilot` today, so a component test for `WorkflowStepRuntimePanel` was not added (would require introducing test infra, out of this task's scope) — recorded as a follow-up.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-181
change_type: bugfix
summary: Add GET /client/workflow-runs/{runId}/steps-runtime backed by workflowStore.LoadRunSteps, and a new Flow-mode-only WorkflowStepRuntimePanel in the desktop right sidebar, so Flow mode surfaces Go's authoritative step/retry/approval state instead of looking like plain chat
# --->8---
