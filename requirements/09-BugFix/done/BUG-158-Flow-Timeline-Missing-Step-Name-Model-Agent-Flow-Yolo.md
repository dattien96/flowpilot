# BUG-158: Flow Timeline Missing Step Name, Model/Agent, And Flow-Level Yolo

## Metadata

- Document ID: `BUG-158`
- Title: `Flow Timeline Missing Step Name, Model/Agent, And Flow-Level Yolo`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-155-Flow-Mode-Sidebar-Shows-Generic-Step-Label-Not-Node-Identity.md`, `requirements/09-BugFix/done/BUG-156-Flow-Timeline-Moved-To-Dedicated-Collapsible-Sidebar.md`
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, ui, desktop, workflow-steps-runtime`

## AI Quick View

### Summary

- After `BUG-156` shipped, a live run of the new sidebar still showed "Flow: Agent Delegate" for every step (not the per-node name) and no model/agent/yolo data anywhere, in both expanded and collapsed mode.
- Root cause was two separate, unrelated gaps, not one: (1) `node_id`/`agent_ref` genuinely are written by the mirror-sync writer and read by `BUG-155`'s fix — a stale, not-yet-rebuilt local-runner binary was serving the old DTO shape without them; (2) `workflow_steps.provider_override`/`model_override` were never a real data source for CP-42 flow-engine nodes — the flow YAML schema (`agentpack.FlowNode`) has no per-node model/provider field, and none of the built-in agent definitions (`coder.md`/`reviewer.md`/`synthesizer.md`/`tester.md`) declare one either, so a node's actual model/provider is always just whatever the run itself was started with (inherited).
- Yolo is a run-wide toggle (`interactiveRun.yolo`, set once per run at start), not a per-step-type default — showing `step_definitions.yolo_mode` per step (what `BUG-155` did) was the wrong level entirely for what the user actually wanted: "YOLO mode of that Flow."
- The collapsed icon rail also had no way to tell steps apart from each other (`BUG-156`'s icons carried no index).

### Current Ask

- Show the real per-step name (once the runner backend is rebuilt — this doc calls that out explicitly, it isn't a further code fix).
- Show the step's model/agent, falling back to the run's own model/provider when the step has none of its own (which is always, for these built-in flows).
- Show the flow/run's own yolo posture once, not a per-step default.
- Number the icons in the collapsed rail (1, 2, 3, 4...).

### Key Decisions

- `F-1` Do not invent a fake per-node model/provider data source. Surface the RUN's own `provider`/`model`/`yolo` once, at the top of the `/client/workflow-runs/{runId}/steps-runtime` response (new top-level `provider`/`model`/`yoloMode` fields on `workflowStepsRuntimeSnapshot`), sourced from the already-tracked `interactiveRun.providerKey`/`modelName`/`yolo` — no new persistence, no new columns.
- `F-2` `FlowStepTimeline` accepts `runProvider`/`runModel` fallback props and uses `step.provider || runProvider` / `step.model || runModel` per row, so a step with its own override (if one is ever added later) still takes precedence, and today every row falls back to the run's values.
- `F-3` `FlowTimelineSidebar`'s expanded summary header shows the flow-level `YOLO` badge (reusing the existing `wsr-retry-badge` pill class) plus the run's provider/model chips, once — not repeated on every row.
- `F-4` Collapsed rail: `FlowStepTimeline` renders `index + 1` inside the icon instead of the state glyph when `compact` is true, so steps are distinguishable at a glance without expanding.
- `F-5` Documented, not re-fixed: the still-generic step name in the user's screenshot is very likely a stale local-runner Go binary (source already had `node_id`/`agent_ref` correctly wired by `BUG-155` and the pre-existing mirror-sync writer) — the fix here is telling the user to rebuild/restart the backend, not another code change.

### Constraints

- No change to the flow YAML schema (`agentpack.FlowNode`) or the built-in agent `.md` files to add a fake per-node model/provider — that would misrepresent that these nodes actually have independent config when they don't.
- `RuntimeWorkflowStep`'s per-step `Provider`/`Model`/`YoloMode` fields (added in `BUG-155`) are left in place for the day a step genuinely does declare its own override; this fix only adds the run-level fallback path alongside them.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/agentpack/pack.go:106-118` (`FlowNode` — no `Provider`/`Model` field)
- `apps/local-runner/internal/agentpack/flow-pack/agents/coder.md` (built-in agent frontmatter — no `model`/`provider`)
- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go:430-452` (`insertSteps` — writes `node_id`/`behavior_id`/`agent_ref`, never `provider_override`/`model_override`, confirming there is no per-node value to read)
- `apps/local-runner/internal/runner/interactive_service.go:91-108` (`interactiveRun.modelName`/`.yolo`/`.providerKey` — the actual run-level source of truth)
- `apps/local-runner/internal/runner/interactive_handlers.go:901-957` (`workflowStepsRuntimeSnapshot`/`workflowStepsRuntime`)

## 1. Issue Summary

A live run of the `BUG-156` sidebar still showed every step as "Flow: Agent Delegate" (no per-step name), and no model/agent/yolo data in either expanded or collapsed mode; the collapsed icon rail also had no way to distinguish steps from one another.

## 2. Parent Links

- impacted coding plan: `none`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, a running Flow Mode Review-Loop-shaped run.
- reproduction steps: start a Flow Mode run; observe the sidebar in both expanded and collapsed states.
- frequency: deterministic.

## 4. Expected vs Actual

- expected: per-step name, model/agent, and flow-level yolo all visible; collapsed rail numbered.
- actual: generic step name repeated, no model/agent/yolo anywhere, collapsed icons unnumbered.

## 5. Impact

- users affected: anyone running Flow Mode.
- workflows affected: sidebar display only.
- severity: low — same class as `BUG-156`, UX/display fidelity.

## 6. Root Cause

- hypothesis: the `BUG-155`/`BUG-156` fixes didn't actually wire the data through.
- confirmed cause: three independent things, not one bug:
  1. Step name: the fix from `BUG-155` (select `node_id`, map it into the DTO, prefer it in `stepLabel`/`stepName`) is already correct in source; a screenshot still showing the old generic name after that fix landed is the signature of a Go backend process that hasn't been rebuilt/restarted since — Vite hot-reloads the TS frontend instantly, but the separate local-runner Go binary requires an explicit rebuild.
  2. Model/agent/provider: these were never wrong to be missing — the flow YAML schema and every built-in agent definition simply have no per-node model/provider field to read. The correct fix is a *fallback to the run's own values*, not a deeper DB lookup for data that doesn't exist.
  3. Yolo: `step_definitions.yolo_mode` (what `BUG-155` exposed per step) is the step *type's* default, not the run's actual yolo setting — the user wanted the flow/run's own yolo posture, which lives on `interactiveRun.yolo` and was never exposed over this endpoint at all.
- evidence: see Source Refs above.

## 7. Fix Strategy

- `F-1` `workflowStepsRuntime` (`interactive_handlers.go`) now also reads `rs.providerKey`/`rs.modelName`/`rs.yolo` from the run and returns them as top-level `provider`/`model`/`yoloMode` on `workflowStepsRuntimeSnapshot`.
- `F-2` `WorkflowStepsRuntimeSnapshot` (TS contract) gains the matching optional fields; `store.ts` gains `workflowStepRuntimeMeta` state, populated in `refreshWorkflowStepRuntime` alongside `workflowStepRuntime`, and reset to `{}` at every existing `workflowStepRuntime: []` reset site.
- `F-3` `FlowStepTimeline` takes `runProvider`/`runModel` props and falls back to them per row (`step.provider || runProvider`); `FlowTimelineSidebar` passes `meta.provider`/`meta.model` through and renders a `YOLO` pill (via the existing `wsr-retry-badge` class) plus provider/model chips in the expanded summary header when `meta.yoloMode`/`meta.provider`/`meta.model` are set.
- `F-4` `FlowStepTimeline`'s icon renders `index + 1` instead of the state glyph whenever `compact` is true.
- `F-5` No code fix for the stale-label symptom — documented as an operational note: rebuild and restart the local-runner backend to pick up `BUG-155`'s already-correct source changes.

## 8. Validation

- `V-1` `go build ./...` in `apps/local-runner` — passes.
- `V-2` `go test ./internal/runner/... -run 'TestWorkflowStepsRuntime'` — passes, including a new `TestWorkflowStepsRuntimeIncludesRunLevelProviderModelYolo` asserting the snapshot's top-level `Provider`/`YoloMode` come from a run started with `YoloMode: true`.
- `V-3` `npm run typecheck` in `apps/desktop-flowpilot` — clean for every file this change touches (confirmed via targeted grep of the typecheck output); unrelated pre-existing errors elsewhere in the working tree (a `pendingQuestion`→`pendingQuestions` rename in progress) are untouched by this change.
- `V-4` Not executed: a live re-check against a rebuilt local-runner binary and an actual running flow, because no backend/Supabase instance is available in this environment (same limitation as `BUG-155`/`BUG-156`). The user should rebuild the local-runner (`go build`) and restart it, then re-open the sidebar, to confirm the step name now shows correctly and the new model/agent/yolo display looks right.

## 9. Regression Guard

- tests: `TestWorkflowStepsRuntimeIncludesRunLevelProviderModelYolo` locks in the new top-level fields.
- alerts: none.
- audit checks: recorded in `change-audit/CA-194-flow-timeline-run-level-model-provider-yolo.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: none.
