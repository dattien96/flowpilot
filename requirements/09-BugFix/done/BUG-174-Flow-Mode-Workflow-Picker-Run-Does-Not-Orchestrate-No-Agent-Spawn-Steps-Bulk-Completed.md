# BUG-174: Flow-Mode Workflow-Picker Run Does Not Orchestrate (No Agent Spawn, Step Stuck At 1, Steps Bulk-Completed)

## Metadata

- Document ID: `BUG-174`
- Title: `Flow-Mode Workflow-Picker Run Does Not Orchestrate (No Agent Spawn, Step Stuck At 1, Steps Bulk-Completed)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`, `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-173-Flow-Step-Timeline-Expanded-Rail-Missing-Step-Numbers.md`, `requirements/09-BugFix/done/BUG-172-Desktop-Blank-Black-Screen-On-Uncaught-Render-Error-In-Flow-Mode-Approval.md`, `requirements/09-BugFix/done/BUG-169-Spawned-Agents-Not-Visible-In-Agents-Panel-Flow-Mode.md`
- Replaces: `none`
- Tags: `agent-flow-engine, runner, desktop, flow-mode, orchestration, review-loop, cp-42`

## AI Quick View

### Summary

- Running a "review-loop" flow in Flow Mode via the **workflow picker** (`launchMode: "workflow"`): the main/hub agent does all the work inline (writes code, tests, commits), NO coder/reviewer sub-agents are spawned, the step timeline stays stuck on step 1, and at the end all 4 steps flip to DONE at once — inconsistently (screenshot shows step 1 "Coder" still RUNNING while downstream Reviewer/Reviewer/Hub show DONE, an impossible ordering).
- Verified root cause: the workflow-picker launch uses the **legacy CP-19 workflow-step catalog path**, which seeds steps from `ListWorkflowSteps` but is fully disjoint from the CP-42 flow-pack executor (`flowRef` → `startResolvedFlow` → cohort spawn). No `flowRef` is sent for this launch path, so the executor never engages; and the executor was never wired to the step-runtime timeline in the first place.
- Three symptoms, three confirmed mechanisms (see §6). This is **incomplete/unconverged CP-42 work**, not a regression: CP-36 P-5 deprecated per-step execution and CP-42 (draft, in-progress, untracked `behaviors/`) is mid-migration to the flow-pack model, but the workflow-picker launch was never migrated onto the executor.
- Fixed (runner-only): the workflow-picker launch now bridges to the flow executor (`resolveWorkflowFlowRef` resolves the run's `workflowID` against the flow definition store — `GetByRef` accepts the mirror row's UUID and normalizes it to the canonical `flowRef`), and the executor now drives the step-runtime timeline node-by-node while the legacy bulk `PlanWorkflowProgress` is gated off for these runs. Unit-tested against the in-memory store the local runner actually uses; a live runner + provider account is still needed to confirm the full cohort/synthesis loop end-to-end (see §8).

### Current Ask

- Route a Flow-Mode workflow-picker run of a built-in orchestration flow through the CP-42 flow executor so the cohort actually spawns, and wire the executor's node lifecycle to the step-runtime timeline so steps advance node-by-node instead of being bulk-completed.

### Key Decisions

- `V-1` A Flow-Mode run whose selected workflow is a built-in orchestration flow (review-loop, etc.) MUST engage the flow executor (spawn entry node → cohort → hub synthesis), not degenerate to a single inline hub turn.
- `V-2` The step-runtime timeline MUST be driven by real flow-node transitions (spawn → RUNNING, complete → DONE) — never by a single post-turn bulk `PlanWorkflowProgress` pass that marks every remaining step DONE regardless of node causality.
- `V-3` (resolved) The bridge from a catalog `workflowID` to a flow-pack `flowRef` is the flow definition store's `GetByRef`, which accepts the mirror row's UUID and returns a record whose `FlowRef` is normalized to the canonical `packId/flowId` — so no new mapping table was needed. Chosen approach (b): runner-side resolution, no desktop change.

### Constraints

- Must not break normal chat runs (single synthetic "chat" step + bulk `Progress` is correct for them) or the existing `flowRef`/bugfix-tab launch path (which already engages the executor).
- Any executor→step wiring must be gated so it only changes behavior for runs the executor actually drives (e.g. `len(rs.activeFlowNodes) > 0`), leaving non-executor runs on their current `Progress` behavior.
- GitNexus MCP tools were unavailable this session for the mandated impact analysis; the touched symbols (`startTurn`, `orchestrator.Progress`, `startResolvedFlow`, `tryAdvanceFlowFromNode`, `createRun`) are HIGH-blast-radius — impact analysis should be run before editing them.
- Cannot be verified live in this environment (no runner + provider account).

### Open Questions

- Live end-to-end confirmation (a real cohort spawning + synthesizing against a provider) is still pending — the fix is unit-tested against the in-memory store the local runner uses, but no runner + provider account was available this session to exercise the full loop. The user should run a workflow-picker review-loop and confirm the coder/reviewer agents now appear and the step timeline advances node-by-node.
- A Supabase-backed step store (not the local runner's in-memory `fakeWorkflowStore`) does not implement `workflowRunSeeder`, so the node-based reseed is a no-op there and the timeline would keep its catalog steps. Not a concern for the reported local-runner scenario; noted for a future hosted deployment.
- Whether the legacy catalog-workflow path should survive at all under CP-42, or whether Flow Mode launches should route exclusively through flow-pack flows, remains a CP-42 design question (this fix bridges rather than replaces).

### Source Refs

- `apps/local-runner/internal/runner/interactive_handlers.go:523-605` (`createRun`: workflow-picker seeds steps from `ListWorkflowSteps`, no `NodeID`/`AgentRef`, no `flowRef`)
- `apps/desktop-flowpilot/src/state/store.ts:1090-1091` (`flowRef` sent only when `chatStartMode === "bugfix"`)
- `apps/local-runner/internal/runner/interactive_service.go:2582-2604` (`startTurn`: `startResolvedFlow` only fires when `in.FlowRef != ""`)
- `apps/local-runner/internal/runner/interactive_service.go:2258` (`orchestrator.Progress` bulk call after every clean turn)
- `apps/local-runner/internal/runner/interactive_service.go:2362-2376`, `:2616` (`markStepRunning` — only the hub turn's own `StepID`)
- `apps/local-runner/internal/runner/workflow_state_machine.go:138-206` (`PlanWorkflowProgress` — bulk-marks every non-terminal step DONE in one pass)
- `apps/local-runner/internal/runner/flow_executor.go` (`startResolvedFlow`, `tryAdvanceFlowFromNode` — spawn/advance, never touch the step store)

## 1. Issue Summary

A review-loop run started from the Flow Mode workflow picker does not orchestrate: the hub agent performs the whole task itself, no coder/reviewer children are spawned, the step timeline never advances past step 1, and when the hub finishes all steps are marked DONE simultaneously and out of causal order.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`, `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- impacted tech design: `none identified` (behavior lives in the runner orchestration code, not a current SD)
- impacted system spec: `none identified`

## 3. Environment and Reproduction

- environment: FlowPilot Desktop + local runner; a "review-loop" built-in flow selected via the **workflow picker** (`launchMode: "workflow"`), YOLO off.
- reproduction steps:
  1. In Flow Mode, pick "review-loop" from the workflow/flow dropdown.
  2. Send a bugfix task prompt (e.g. "add input validation to the parseUserID function").
  3. Observe: the hub agent does everything inline; no sub-agent cards appear; the step timeline shows step 1 RUNNING the whole time; at the end all 4 steps flip to DONE.
- frequency: deterministic for the workflow-picker launch path.

## 4. Expected vs Actual

- expected: the flow spawns a Coder agent, then a Reviewer cohort, coordinated by the Hub, with the step timeline advancing node-by-node (Coder DONE → Reviewers RUNNING → … → Hub DONE).
- actual: no sub-agents spawn; the hub does everything inline; the timeline stays on step 1 then bulk-completes.

## 5. Impact

- users affected: anyone launching a built-in orchestration flow (review-loop and similar) via the Flow Mode workflow picker.
- workflows affected: the entire multi-agent orchestration value proposition for the workflow-picker launch path; the run may still "work" (hub does the task) but the advertised cohort/review loop never runs.
- severity: high — the core Flow Mode feature does not orchestrate for this launch path, and the timeline actively misrepresents what happened.

## 6. Root Cause

Three symptoms, three confirmed mechanisms (all verified by direct code inspection this session):

- **(a) No sub-agents spawned.** The cohort only spawns when `startTurn` sees a non-empty `in.FlowRef` on the first turn and calls `startResolvedFlow` (`interactive_service.go:2587-2603`). The desktop only sends `flowRef` when `chatStartMode === "bugfix"` (`store.ts:1091`); a workflow-picker launch (`launchMode: "workflow"`) does not, so `startResolvedFlow` never runs, `activeFlowEdges`/`activeFlowNodes` stay empty, and `flow_executor.go` is a no-op. Additionally, the workflow-picker run is seeded from the **legacy workflow catalog** (`createRun` → `ListWorkflowSteps`, `interactive_handlers.go:538-563`), which is disjoint from the flow-pack executor and does not populate `NodeID`/`AgentRef` on the steps — so even the data needed to drive per-node spawns is absent on this path.
- **(b) Stuck on step 1.** Only `markStepRunning` (`interactive_service.go:2362-2376`, called from `startTurn` at `:2616`) writes a step to RUNNING, and only for the hub turn's own `in.StepID`. The flow executor's node completions never emit step transitions, so no node-by-node progression exists.
- **(c) All DONE at once, out of order.** After every clean provider turn, `startTurn` calls `orchestrator.Progress` (`:2258`) → `PlanWorkflowProgress` (`workflow_state_machine.go:138-206`), which walks every non-terminal step and marks each DONE in a single pass, independent of node causality. Step 1 can still read RUNNING (its `markStepRunning` write) while 2–4 flip to DONE, producing the impossible ordering in the screenshot.

This is unconverged CP-42 work, not a regression: `behaviors/` is untracked, CP-42 is `todo`/`draft`, CP-36 P-5 deprecated the per-step path, and the workflow-picker launch was never migrated onto the flow executor.

## 7. Fix Strategy

All runner-side. Every new behavior is gated behind a per-run `flowEngineDriven` flag set **only** for the workflow-picker auto-resolved path, so the explicit chat/flowRef (bugfix-tab) path and plain workflows keep their exact existing behavior.

- `F-1` **(implemented)** Bridge the workflow-picker launch to the executor. `handleStartTurn` calls `resolveWorkflowFlowRef(ctx, runID)` when no `flowRef` was sent (`interactive_handlers.go`); that helper (`flow_executor.go`) resolves the run's `workflowID` via `FlowDefinitionResolver.ResolveFlowRef` — whose store `GetByRef` accepts the mirror row's UUID and returns a record whose `FlowRef` is normalized to the canonical `packId/flowId`. If the workflow resolves to a flow with a spawnable entry node, the handler adopts that `flowRef` and calls `markFlowEngineDriven(runID)`, so the run engages the exact same `startResolvedFlow` path an explicit `flowRef` uses.
- `F-2` **(implemented)** Wire the executor to the step timeline (`flow_step_runtime.go`, gated on `flowEngineDriven`): `startResolvedFlow`/`startInlineEntryChain` reseed the step list from the flow's nodes (one step per node, `ID == NodeID`) and mark the entry node RUNNING; `tryAdvanceFlowFromNode` marks the completed node DONE and its forward targets RUNNING; the review-cohort join (`interactive_service.go` `EventMessageCompleted` handler) marks each reviewer DONE and the inline hub node RUNNING; `applyFlowControl("done")` marks the hub node DONE and the run DONE (`markFlowRunComplete`); `applyFlowControl("continue")` resets downstream nodes to PENDING and re-runs the entry node for the next round.
- `F-3` **(implemented)** Gate the bulk `orchestrator.Progress` call (`interactive_service.go:2258`) so it is skipped for `flowEngineDriven` runs — the executor owns their transitions. Normal chat and non-executor runs are unaffected.
- `F-4` **(implemented)** `setFlowStepStatus` transitions a node's step by id via `WorkflowStore.ApplyStepTransition` (stamping RUNNING/DONE/PENDING timestamps); `reseedFlowStepRuntime` rebuilds the list via the `workflowRunSeeder` cast (the local runner's in-memory store implements it). Both best-effort — a step-timeline write never breaks the flow's orchestration.

## 8. Validation

- `V-1` **(done)** New unit tests in `flow_step_runtime_test.go`, all passing: `TestReseedFlowStepRuntimeSeedsOneStepPerNode`, `TestSetFlowStepStatusTransitionsByNodeID`, `TestTryAdvanceFlowMarksCompletedDoneAndTargetsRunning` (coder→reviewer cohort advances node-by-node, no bulk flip), `TestFlowEngineDrivenRunSkipsBulkProgress` (control proves a non-flow run bulk-completes while a `flowEngineDriven` run does not), `TestResolveWorkflowFlowRefAdoptsMirroredFlow` and `TestResolveWorkflowFlowRefBailsForPlainRun`.
- `V-2` **(done)** Full flow/workflow/orchestration/step-runtime suite green: `go test ./internal/runner/ -run 'Flow|Workflow|Orchestrat|Cohort|Coder|Reviewer|Progress|StepRuntime|Advance|Resolve'` → 284 passed. `go build ./internal/runner/` clean.
- `V-3` **(done)** The ~15 unrelated failures in the full package run were confirmed pre-existing/environment-specific by stashing this change and re-running the same subset on a clean tree (identical failures): codex-resume tests (need a real codex binary), Windows path/home tests, and `TestSkillsMerge*WithPrecedence`/`TestStartInteractiveAuthLaunchesFromWorkspace` (already flagged pre-existing in BUG-169/170).
- `V-4` **(not executed)** A live workflow-picker review-loop run against a real provider account, to confirm the full cohort spawn + synthesis + step advancement end-to-end. No runner + provider account was available in this environment (see Open Questions).

## 9. Regression Guard

- tests: `flow_step_runtime_test.go` guards node-by-node advancement, the `flowEngineDriven` Progress gate (with a control that proves a non-flow run still bulk-completes), and the `workflowID→flowRef` bridge; existing `flow_executor_test.go` / orchestration tests (284 total) confirm the executor spawn/advance path is unregressed.
- alerts: none.
- audit checks: recorded in `change-audit/CA-211-flow-mode-workflow-picker-executor-and-step-wiring.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: `CP-42` should capture the catalog-workflow → flow-pack-executor convergence decision (`V-3`) and the executor→step-timeline wiring as explicit plan items; `CP-36` P-5's deprecation of the per-step path should note that the workflow-picker launch still routes through it and needs migration.
- notes left unchanged on purpose: the numbering cosmetic (`BUG-173`) and the black-screen resilience fix (`BUG-172`) are separate, already-landed issues from the same report thread.
