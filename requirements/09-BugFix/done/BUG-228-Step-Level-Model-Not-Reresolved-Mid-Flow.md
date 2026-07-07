# BUG-228: Agent-Delegate Flow Node Ignored Its Own Step Model Mid-Flow

## Metadata

- Document ID: `BUG-228`
- Title: `Agent-Delegate Flow Node Ignored Its Own Step Model Mid-Flow`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [Task-183: User-Owned Model/Provider In Flow Mode](../../08-Task/todo/Task-183-User-Owned-Model-Provider-Resolution-Across-Chat-And-Flow.md), [BUG-165: Implement Step > Flow > Project > Default Model Resolution](./BUG-165-Implement-Step-Flow-Project-Default-Model-Resolution.md), [BUG-161: Add Coder Reviewer Step Types And Fix Reasoning Override](./BUG-161-Add-Coder-Reviewer-Step-Types-And-Fix-Reasoning-Override.md), [BUG-171: Flow Run Provider Not Reconciled With Resolved Model](./BUG-171-Flow-Run-Provider-Not-Reconciled-With-Resolved-Model.md)
- Child Documents: `none`
- Related Documents: [BUG-227: Flow Mode Main Card Shows Pre-Run Catalog Model Instead Of Resolved Model](./BUG-227-Flow-Mode-Main-Card-Shows-Pre-Run-Catalog-Model-Instead-Of-Resolved-Model.md), [BUG-229: Single-Step Launch Falls Back To Project Model](./BUG-229-Single-Step-Launch-Falls-Back-To-Project-Model.md), [BUG-230: Navigator Catalog Mapping Drops Model And YoloMode](./BUG-230-Navigator-Catalog-Mapping-Drops-Model-And-Yolo-Mode.md)
- Replaces: `none`
- Tags: `agent-flow-engine, workflow-engine, model-resolution, flow-executor`

## AI Quick View

### Summary

- User configured the "Reviewer" step type's `step_definitions.model` to Claude Sonnet in Settings › Workflows › Steps, then ran the built-in "Review Loop" workflow (resolved to Claude Haiku at run start). Both `reviewer_correctness` and `reviewer_security` steps executed on Claude Haiku instead of the configured Sonnet — the step's own model had no effect once the flow was already running.
- Initially filed as a documented, deliberately-unfixed known limitation (this doc's original content): `spawnChildRun`, the sole spawn path for every flow-graph `agent.delegate` node, only reads the (intentionally model-free) agent-definition markdown or inherits the parent run's single resolved model — it never looked up a node's own `step_definitions.model`.
- The user then gave the exact intended design: **within one flow, an agent node (spawns a separate child run) can run its own provider/model; an inline node (executes within the parent's own turn) always uses the flow's model.** This is now implemented — see Key Decisions.

### Current Ask

- An `agent.delegate` flow node resolves its own model from its role's purpose-named `step_definitions` row, independent of the run's own baseline model. An inline (`hub.inline`) node is unaffected — it has no separate spawn to begin with.

### Key Decisions

- `F-1` Added `resolveFlowNodeModel(ctx, node)` (`flow_executor.go`): maps a node's `agent:` reference to its bare role (`agents/reviewer.md` → `reviewer`, reusing `flowNodeAgentName`'s existing derivation) and looks up the catalog row `flow-agent-delegate-<role>` — the same purpose-named `step_definitions` rows (`flow-agent-delegate-coder`, `flow-agent-delegate-reviewer`) the manual workflow builder's "Add step" dropdown already offers as "Flow: Coder" / "Flow: Reviewer" (`BUG-161`). Returns `""` when the role has no such row or no model configured, so the node transparently falls back to the pre-existing inherit-from-parent behavior.
- `F-2` Wired `resolveFlowNodeModel` into all three `spawnChildRun` call sites in `flow_executor.go` (`startResolvedFlow`'s entry-node spawn, `startInlineEntryChain`'s delegate-target spawn, `tryAdvanceFlowFromNode`'s auto-advance spawn) via a new `SpawnAgentInput.Model` field (internal-only, not wire-decoded — same convention as `AgentDefOverride`/`UIInitiated`).
- `F-3` `spawnChildRun` (`interactive_service.go`) now treats `in.Model` as the top-priority tier for both the child's model AND its derived provider — above the agent definition and above parent inheritance — using the same "model is authoritative" pattern `BUG-171` established for a run's own provider derivation, so a node whose role resolves to a different provider than the parent (e.g. a Claude hub delegating to a Codex-configured role) is stamped with the correct provider, not the parent's.
- `F-4` Both cohort siblings sharing one agent role (`reviewer_correctness`, `reviewer_security`, both `agents/reviewer.md`) resolve to the **same** `flow-agent-delegate-reviewer` row by design — this is a per-role override, not a distinct override per individual graph node.
- `F-5` An inline node (`run: inline`, `behavior: hub.inline`, e.g. `synthesis`) never calls `spawnChildRun` at all — it executes as the parent run's own turn using the run's already-resolved model, so no change was needed there.
- `F-6` `SS-05` §3 and `SD-06` §6.2.1/§6.3 updated to document this per-node override alongside the run-level resolution chain.
- `F-7` **Display follow-up, round 1**: the execution fix (`F-1`..`F-3`) made the real spawned agent run on its own resolved model, but the desktop's step-timeline sidebar still showed the run's baseline model for every row, because `flowStepRowsFromNodes` (the flow-engine step-runtime seeder) never populated a row's `Provider`/`Model` and `WorkflowStepPatch` had no way to patch them after the fact — every row stayed blank, and the desktop UI fell back to displaying the run-level baseline for every step regardless of that step's own posture. Added `Provider *string`/`Model *string` to `WorkflowStepPatch`, a `setFlowStepPosture` helper, and a `stampFlowNodePosture` helper (`flow_executor.go`) that resolves a node's effective provider/model (its own `flow-agent-delegate-<role>` row, else the run's baseline — the same fallback `spawnChildRun` uses) and patches it onto that node's step-timeline row right after each of the three `setFlowStepStatus(..., StepStatusRunning)` calls.
- `F-8` **Display follow-up, round 2**: `F-7` only stamped a node's posture once it actually spawned (RUNNING) — a `PENDING` row (every not-yet-reached step, for most of a run's lifetime) still showed no posture and fell back to the baseline. Extracted `resolveFlowNodeProviderModel(ctx, parentRunID, node) (provider, model string)` out of `stampFlowNodePosture` and reused it inside `flowStepRowsFromNodes` (converted from a free function to a method) so every row — regardless of status — gets its correct eventual posture resolved **at seed time**, before any node has been spawned at all.
- `F-9` **Deadlock found and fixed during `F-8` verification**: `flowStepRowsFromNodes` now calls `resolveFlowNodeProviderModel`, which acquires `s.mu` — but `reconstructRun` (`interactive_resume.go`, the resume-after-restart path) called `reseedFlowStepRuntimeForResume` (which reaches `flowStepRowsFromNodes`) while STILL HOLDING `s.mu.Lock()`, deadlocking on the non-reentrant mutex for any resumed flow-engine run. Confirmed concretely: the first post-`F-8` full regression run stalled at ~472/1058+ tests and was killed by `go test`'s 10-minute default timeout, dumping goroutines parked on `chan receive` for 9 minutes. Fixed by moving `s.mu.Unlock()` earlier in `reconstructRun`, before the reseed call.

### Follow-Up Fix (display parity)

- **Symptom, round 1**: after `F-1`..`F-6` shipped, the user reported the "Flow: Reviewer" step configured to Claude Sonnet in Settings still showed `claude-haiku` in the desktop's step-timeline sidebar (`1/4 steps` panel) during a live run.
- **Root cause, round 1**: that panel reads `WorkflowStepRuntimeDTO.provider`/`.model` per row, sourced from `RuntimeWorkflowStep.Provider`/`.Model` — fields the flow-engine step seeder never set (unlike the classic planner, which seeds them statically from the catalog up front) and that had no patch path to be set later. The *execution* fix was already correct (verified via the actual spawned child's `run.modelName`); only the *read model* backing this specific display was stale.
- **Fix, round 1**: `F-7` — `stampFlowNodePosture`, called right after each node's `setFlowStepStatus(..., StepStatusRunning)` transition.
- **Symptom, round 2**: after `F-7` shipped, the user's next screenshot showed the SAME stale `claude-haiku` — but this time for `reviewer_correctness`/`reviewer_security` rows still in `PENDING` status (not yet spawned).
- **Root cause, round 2**: `F-7` only stamps a node's posture once it is actually spawned (transitions to RUNNING). A `PENDING` row — seeded by `flowStepRowsFromNodes` before anything spawns — never had its `Provider`/`Model` set at all, so the desktop fell back to the run's baseline for every not-yet-started step, which is exactly what a user watching the timeline sees for most of a run's lifetime (each step is `PENDING` until its turn comes).
- **Fix, round 2**: `F-8` — `flowStepRowsFromNodes` itself (now a method, `(s *InteractiveService) flowStepRowsFromNodes`) resolves each node's posture via the same `resolveFlowNodeProviderModel` helper **at seed time**, so every row — `PENDING`, `RUNNING`, or `DONE` — shows its correct eventual posture from the moment the flow starts, not only after that specific node has been spawned. `stampFlowNodePosture` (round 1) still re-stamps at RUNNING time as a redundant confirmation/regression guard, not the sole source of truth anymore.

### Constraints

- Do not change the run-level resolution chain (`Step > Flow > Project`, `BUG-165`/`BUG-229`) — it still resolves exactly one baseline model/provider for the run at start; this fix only adds a node-level override on top of that baseline.
- Do not add `model:`/`provider:` to built-in pack YAML or agent markdown (`CP-42` `P-1`, Task-183 constraint) — the override is sourced entirely from the `step_definitions` catalog, never from pack files.
- Do not affect reasoning effort resolution — `step_definitions` has no reasoning-effort tier (`SD-06` §4.2/§6.2), and this fix does not add one; only model/provider are node-overridable.
- Do not affect YOLO mode — per the user's own restated definition, a normal flow's YOLO posture is Flow-level only, uniformly for every step in the run; this fix does not introduce a per-node YOLO override.

### Open Questions

- None — the two candidate directions this doc originally posed (`Q-1`/`Q-2`) are resolved: the user confirmed direction (a), scoped exactly to agent nodes reading their own role's `step_definitions.model`, with inline nodes explicitly excluded.

### Source Refs

- `apps/local-runner/internal/runner/flow_executor.go` (`resolveFlowNodeModel`, `resolveFlowNodeProviderModel`, `stampFlowNodePosture`, and their call sites in `startResolvedFlow`, `startInlineEntryChain`, `tryAdvanceFlowFromNode`)
- `apps/local-runner/internal/runner/agent_orchestrator.go` (`SpawnAgentInput.Model`)
- `apps/local-runner/internal/runner/interactive_service.go` (`spawnChildRun` — `in.Model` priority for both model and provider derivation)
- `apps/local-runner/internal/runner/flow_step_runtime.go` (`setFlowStepPosture`, `flowStepRowsFromNodes` — now a method, resolves posture at seed time)
- `apps/local-runner/internal/runner/interactive_resume.go` (`reconstructRun` — `F-9` deadlock fix: `s.mu.Unlock()` moved before the `reseedFlowStepRuntimeForResume` call)
- `apps/local-runner/internal/runner/workflow_state_machine.go` (`WorkflowStepPatch.Provider`/`.Model`)
- `apps/local-runner/internal/runner/workflow_store.go` (`fakeWorkflowStore.ApplyStepTransition` — applies the new patch fields; this is also the real production `WorkflowStore`, embedded by `localFileSessionStore`)
- `apps/local-runner/internal/runner/flow_executor_test.go` (`TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel`)
- `apps/local-runner/internal/runner/workflow_step_runtime_test.go` (`TestWorkflowStepsRuntimeReflectsPerNodeModelOverride`)
- `supabase/migrations/20260702120000_split_flow_agent_delegate_coder_reviewer.sql` (the purpose-named `step_definitions` rows this fix reads)
- `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md` §3
- `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md` §6.2.1, §6.3

## 1. Issue Summary

An `agent.delegate` flow-graph node (e.g. a reviewer step) always ran on the flow run's single baseline model, silently ignoring its own role's configured `step_definitions.model` whenever that role's model differed from the run's baseline — making the per-step Model field in Settings › Workflows › Steps look broken for any step used mid-flow.

## 2. Parent Links

- impacted coding plan: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- impacted tech design: `SD-06-AI-Provider-Integration.md` §6.2.1 (new), §6.3 (updated)
- impacted system spec: `SS-05-Workflow-Ai-Provider.md` §3 (updated)

## 3. Environment and Reproduction

- environment: local-runner, any multi-step Flow-mode workflow with an `agent.delegate` role's `step_definitions.model` configured differently from the run's own resolved model (e.g. built-in "Review Loop": reviewer role set to Claude Sonnet, run's own resolved model Claude Haiku).
- reproduction steps (pre-fix): set a role's `step_definitions.model` (e.g. via the "Flow: Reviewer" catalog entry) to a value different from the run's baseline; run the workflow; observe the role's spawned agent card in the Agents panel.
- frequency: deterministic — every occurrence of that role in every flow run using it was affected.

## 4. Expected vs Actual

- expected: an agent-delegate node's spawned child runs on its own role's configured model when one exists.
- actual (pre-fix): every agent-delegate node in a flow run shared the run's single baseline model regardless of its own role's configuration.

## 5. Impact

- users affected: anyone configuring per-role models for flow steps (e.g. a stronger review model than the coding model) in a multi-step Flow-mode workflow.
- workflows affected: any multi-step Flow-mode workflow where roles are expected to run on different models/providers from each other.
- severity: medium — no crash, but a configured setting silently had no effect, which read as a bug rather than a documented boundary (which is exactly how this was reported).

## 6. Root Cause

- confirmed cause: `spawnChildRun`, the sole spawn path for every flow-graph node, had no plumbing to accept or look up a per-node/per-role model — `SpawnAgentInput` carried no model field, and `flow_executor.go`'s three spawn call sites never resolved one.
- evidence: see Source Refs.

## 7. Fix Strategy

- `F-1`..`F-9` as described in Key Decisions.

## 8. Validation

- `V-1` New regression test `TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel` (`flow_executor_test.go`): drives the full `startResolvedFlow` → coder completion → auto-advance → reviewer cohort spawn path with a catalog carrying a `flow-agent-delegate-reviewer` row set to `claude-sonnet`, and asserts both reviewer children resolve to `provider=claude, model=claude-sonnet` while the coder child (no matching row) still inherits the run's own `provider=codex, model=gpt-5.4-mini` — proving this is a per-role override, not a blanket switch. Passed 20/20 on repeat runs (no `-race` available in this environment — cgo disabled).
- `V-2` New regression test `TestWorkflowStepsRuntimeReflectsPerNodeModelOverride` (`workflow_step_runtime_test.go`, `F-7`/`F-8`): drives the same flow through the HTTP `/steps-runtime` endpoint (with `markFlowEngineDriven` set, mirroring a real workflow-picker launch) and asserts (a) the reviewer rows already carry `provider=claude, model=claude-sonnet` immediately after `startResolvedFlow` returns — whatever status they happen to be in at that instant, since the fake adapter completes turns synchronously and can race the flow ahead before the assertion runs — and (b) the same posture holds once the wait-loop confirms the cohort has left `PENDING`, while `coder`'s row shows the run's own baseline throughout. Passed 20/20 on repeat runs; an earlier draft of this test asserted a specific `PENDING` status at the first checkpoint and was itself flaky (~1/3 runs) purely from that timing assumption, unrelated to production behavior — corrected to check posture regardless of status.
- `V-3` `go build ./...` in `apps/local-runner` — passes. `go vet ./internal/runner/...` — clean.
- `V-4` **Deadlock (`F-9`) confirmed and fixed**: before the `interactive_resume.go` fix, `go test ./internal/runner/...` reliably stalled at ~472/~1087 tests and hit `go test`'s 10-minute default timeout (reproduced 3x, including once with `-v` capturing the goroutine dump naming `reconstructRun`'s call chain). After the fix, `go test ./internal/runner/...` reliably completes in ~90-100s — reproduced 4x with explicit `-timeout 120s`/`180s` to positively rule out a hang, not just measure a lucky fast run. `TestReconstructRunRestoresTrackedFlowTopology`/`TestReconstructRunPreservesUpdatedAt` (the tests exercising the exact deadlocking code path) both pass.
- `V-5` `go test ./internal/runner/...` (post-fix, steady state) — 1058 passed, 15 failed (identical pre-existing, environment-specific failure set already established as baseline this session), 14 skipped.
- `V-6` Targeted regression re-run: `go test ./internal/runner -run 'TestCreateRun|TestE2EReviewLoop|TestStartTurnWithFlowRefEmitsSyntheticTurnCompletedForHubHandoff|TestAutoReinvokeHub|TestWorkflowStepsRuntime|TestCoderCompletion|TestStartResolvedFlow'` — all passed.
- `V-7` Not executed: a live re-check against a running desktop app + real Supabase instance — no backend/live environment available in this session. Two rounds of the user's own live screenshots (`F-7`, then `F-8`) are what surfaced each display gap in turn; a third live re-check after `F-8`/`F-9` is still pending on the user's side.

## 9. Regression Guard

- tests: `TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel` locks in the per-role override and the coder's unaffected inheritance in the same test, so a regression in either direction fails it.
- alerts: none.
- audit checks: recorded in `change-audit/CA-230-agent-delegate-node-per-role-model.md`.

## 10. Follow-Up Document Updates

- upstream docs updated as part of this fix: `SS-05-Workflow-Ai-Provider.md` §3, `SD-06-AI-Provider-Integration.md` §6.2.1 (new), §6.3.
- notes left unchanged on purpose: the run-level `Step > Flow > Project` resolution chain (`BUG-165`/`BUG-229`) is unaffected — it still resolves once, at run start, for the run's own baseline; this fix only adds a node-level override on top of it.
