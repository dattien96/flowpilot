# Task-183: User-Owned Model/Provider In Flow Mode — No Hardcoded Default, Editable Built-in Model

## Metadata

- Document ID: `Task-183`
- Title: `User-Owned Model/Provider In Flow Mode — No Hardcoded Default, Editable Built-in Model`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-02`
- Last Updated: `2026-07-06`
- Parent Documents: [SS-05: Workflow AI Provider & Model](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- Child Documents: `None`
- Related Documents: [BUG-165: Implement Step > Flow > Project > Default Model Resolution](../../09-BugFix/done/BUG-165-Implement-Step-Flow-Project-Default-Model-Resolution.md), [BUG-164: Remove workflow_steps Model/Provider/Reasoning Overrides](../../09-BugFix/done/BUG-164-Remove-Workflow-Steps-Model-Provider-Reasoning-Overrides.md), [BUG-163: Flow Pack Mirror Sync Stamps Codex Provider Override](../../09-BugFix/done/BUG-163-Flow-Pack-Mirror-Sync-Stamps-Codex-Provider-Override.md), [BUG-162: Coder Reviewer Steps Must Default To Claude Haiku](../../09-BugFix/done/BUG-162-Coder-Reviewer-Steps-Must-Default-To-Claude-Haiku.md), [BUG-160: Workflow Step Model Override Forced And Step Identity Hidden](../../09-BugFix/done/BUG-160-Workflow-Step-Model-Override-Forced-And-Step-Identity-Hidden.md), [BUG-171: Flow Run Provider Not Reconciled With Resolved Model](../../09-BugFix/done/BUG-171-Flow-Run-Provider-Not-Reconciled-With-Resolved-Model.md), [Task-179: Settings Flow Pack Authoring UI](./Task-179-Settings-Flow-Pack-Authoring-UI.md), [Task-181: Flow Mode Workflow Step Runtime Sidebar](../done/Task-181-Flow-Mode-Workflow-Step-Runtime-Sidebar.md), [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- Replaces: `None`
- Tags: `ai-providers, workflow-engine, flow-pack, model-resolution, desktop-chat, built-in-flow`

## AI Quick View

### Summary

- The Step > Flow > Project resolution already exists (`BUG-165`), Chat-mode controller selection is already authoritative and gated (`ChatInput.tsx:737`), and per-`workflow_steps` overrides were already removed (`BUG-164`). This task does **not** rebuild those.
- Genuine delta: (1) remove the hard-coded `gpt-5.4` floor and make an unresolved model a **non-runnable, greyed** state in Flow mode instead of a silent default; (2) let a user change a **built-in** flow's main-agent model and YOLO mode by unlocking only `workflows.model_override` + reasoning + YOLO mode while structure stays read-only; (3) confirm Chat sub-agents inherit the controller model; (4) desktop Flow-mode UX: show nothing until a flow is picked, then a main-agent card + greyed/tooltip'd unrunnable entries.
- Model/provider stays fully user-owned and pack-free: built-in agent markdown and flow YAML carry no `model:`/`provider:` (unchanged). Provider always derives from the resolved model.
- **Target the live path only.** `apps/admin-web` is legacy/unused (`BUG-164` F-7) — no admin-web edits. Live path = Go runner + `apps/desktop-flowpilot` + `packages/flowpilot-client-core`.

### Current Ask

- Investigate current code and produce this detailed coding plan. **No code in this turn** — implementation is deferred.

### Key Decisions

- `T-1` Remove the `gpt-5.4` hard floor from Flow-mode resolution; when Step/Flow/Project all yield no model, the run is not started and the flow/step is surfaced as non-runnable (not silently `gpt-5.4`).
- `T-2` Model is mandatory when creating a user flow (`workflows.model_override`) and a step type (`step_definitions.model`) in `WorkflowsSettings.tsx` (form validation).
- `T-3` Unlock only `modelOverride` + `reasoningEffortOverride` + YOLO mode on built-in flows in Settings while every structural field stays `readOnly`; persist without cloning; mirror sync must never clobber the user's model/reasoning/YOLO mode.
- `T-4` Keep the documented resolution (Chat=controller; Flow=`workflows.model_override`; Step=`step_definitions.model`; Project=`projects.default_model`) but replace tier-4 (`gpt-5.4`) with the mandatory/greyed rule.
- `T-5` Chat sub-agents (coder/reviewer/…) inherit the controller model/provider; verify current spawn behavior and keep pack frontmatter model-free.
- `T-6` Desktop Flow mode: show no provider/model until a flow is selected; then render a read-only main-agent card with the resolved model + derived provider.
- `T-7` Flows/steps whose model cannot resolve appear as disabled dropdown entries with an explanatory tooltip; selectable once a model is set.

### Constraints

- Do not add `model:`/`provider:` to built-in pack YAML or agent markdown (CP-42 `P-1`).
- Do not touch `apps/admin-web` (legacy/unused).
- Do not reintroduce per-`workflow_steps` overrides (reverts `BUG-164`); a step's model is its step type's `step_definitions.model`.
- Do not change Chat-mode behavior — the controller passthrough (Case 1) must not regress.
- Mirror sync (`FlowMirrorSyncService.SyncBuiltins`, `insertSteps`) must preserve user-set built-in `model_override`/reasoning/yolo_mode and write only structural + pack-hash/version fields.
- This modifies SS-05/SD-06 truth (removing the hard floor, adding the mandatory/greyed rule, allowing built-in `workflows.model_override` and YOLO mode edits) — those docs must be updated, not just this task file (SS-13 §7.3).

### Open Questions

- `Q-1` With the `gpt-5.4` floor removed, is Project default (`projects.default_model`) the intended last-resort so "unresolved" only occurs when Step, Flow, and Project are all empty — or should Project default also be optional, making unresolved more common?
- `Q-2` Built-ins are `editable === false`; unlocking `workflows.model_override` and YOLO mode for them needs a persistence path that isn't the clone flow and that the Go `WorkflowStore` (reads/patches only today) can write — is a scoped patch endpoint required?
- `Q-3` Does `BUG-171`'s provider/model reconciliation already cover "provider derives from resolved model" for the greyed/unresolved case, or does `T-4` extend it?
- `Q-4` Per-step model re-resolution during a multi-step run is a known limitation (BUG-165 / SD-06 §6.2) — confirm it stays out of scope here.

### Source Refs

- SS-05 §1.2, §2.1, §2.2, §3, §4.2; SD-06 §6, §7, §8.
- BUG-165 `F-1`..`F-5`; BUG-164 `F-1`,`F-2`,`F-6`.
- Code anchors: [interactive_handlers.go createRun](../../../apps/local-runner/internal/runner/interactive_handlers.go), [supabase_catalog_store.go](../../../apps/local-runner/internal/runner/supabase_catalog_store.go), [provider_event.go](../../../apps/local-runner/internal/runner/provider_event.go), [flow_definition_resolver.go:293](../../../apps/local-runner/internal/runner/flow_definition_resolver.go), [WorkflowsSettings.tsx:659](../../../apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx), [ChatInput.tsx:737](../../../apps/desktop-flowpilot/src/components/ChatInput.tsx), [ChatWorkspace.tsx](../../../apps/desktop-flowpilot/src/components/ChatWorkspace.tsx), [store.ts:1030](../../../apps/desktop-flowpilot/src/state/store.ts).

## 1. Goal

Make the Flow-mode model choice fully user-owned with no silent hard-coded fallback, and let a user set the main-agent model of a **built-in** flow without cloning it. Concretely:

- Keep the existing, correct resolution (Chat=controller; Step=`step_definitions.model`; Flow=`workflows.model_override`; Project=`projects.default_model`).
- Replace the `gpt-5.4` hard floor with a mandatory-model rule: if nothing resolves, the flow/step is non-runnable and shown greyed with a tooltip, never silently defaulted.
- Unlock `workflows.model_override` + reasoning + YOLO mode on built-in flows (structure stays read-only), persisted without cloning and preserved across mirror sync.
- Desktop Flow mode shows no model until a flow is picked, then a main-agent card with the resolved model + derived provider.
- Chat sub-agents inherit the controller model.

## 2. Parent Links

- coding plan: CP-42 (`P-2` override precedence, `P-6` Settings editing, `P-7` built-in read-only + clone, `P-9` Chat baseline)
- tech design: SD-06 (AI Provider Integration — §6/§7/§8 resolution), SD-19 (Agent Flow Engine)
- system spec: SS-05 (Workflow AI Provider & Model — §2.1 default, §3 resolution rules)
- specific upstream ids: SS-05 §3 (resolution order), SD-06 §6.2 (once-per-run limitation), BUG-165 `F-2` (the `gpt-5.4` floor this task removes), BUG-164 `F-1`/`F-6`

## 3. Trigger

In the desktop app the user selects provider/model/reasoning in Chat mode; switching to Flow mode hides that controller (correctly — Case 1). But two gaps remain: built-in flows expose no way to change the main agent's model (the Settings editor is fully `readOnly` for built-ins), and when no model is configured the runner silently falls back to the hard-coded `gpt-5.4` (BUG-165 `F-2`), so a built-in effectively forces Codex. The user requires model/provider to depend on the user in every mode, built-ins to be model-tunable, and no silent hard-coded default — an unresolved model should be a visible, blocked state instead.

## 4. Exact Change

- `T-1` In the Go run-start resolution (`createRun`, [interactive_handlers.go](../../../apps/local-runner/internal/runner/interactive_handlers.go)) remove the `gpt-5.4` last-resort assignment for the `WorkflowID != ""` branch; when Step/Flow/Project all resolve empty, do not start the run — return a clear "no model configured" error the desktop can surface. (Chat branch passthrough unchanged.)
- `T-2` Enforce model as a required field on flow create (`workflows.model_override`) and step-type create/edit (`step_definitions.model`) in [WorkflowsSettings.tsx](../../../apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx) — block Save with a clear message when empty; stop seeding `DEFAULT_MODEL` as an implicit value (draft may start empty, but Save requires a choice).
- `T-3` Split the built-in `readOnly` gate (`WorkflowsSettings.tsx:659`, bound at `:677`) so a built-in flow's Model override + Reasoning effort + YOLO mode inputs stay enabled while Name/Description/structure remain disabled. Persist the change for built-ins without the clone flow (resolve Q-2 for the write path). Ensure `FlowMirrorSyncService.SyncBuiltins`/`insertSteps` ([flow_definition_resolver.go:293](../../../apps/local-runner/internal/runner/flow_definition_resolver.go), [supabase_workflow_flow_store.go](../../../apps/local-runner/internal/runner/supabase_workflow_flow_store.go)) never overwrite user-set model/reasoning/yolo_mode.
- `T-4` Confirm the resolution tiers stay as documented and only the tier-4 default changes (to the mandatory/greyed rule); reconcile provider derivation with BUG-171 so the resolved model's provider is used consistently.
- `T-5` Verify Chat sub-agent spawns inherit the controller model/provider; if they don't, route the parent run's `modelName`/provider into the spawn (agent orchestrator path). Keep built-in agent markdown frontmatter free of `model:`/`provider:`.
- `T-6` Desktop Flow mode ([ChatWorkspace.tsx WorkflowControlPanel](../../../apps/desktop-flowpilot/src/components/ChatWorkspace.tsx)): render no provider/model until a flow is selected; on selection show a read-only "Main agent" card with the resolved model + derived provider. Confirm `startRun` still sends no chat model for workflow/step mode ([store.ts:1030-1041](../../../apps/desktop-flowpilot/src/state/store.ts)) so the runner remains the source of truth.
- `T-7` In the Flow/step dropdowns, render entries whose model cannot resolve as disabled with a hover tooltip (e.g. "No model set — choose a model in Settings › Workflows before running"); enable automatically once a model is set.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/interactive_handlers.go` (`createRun` resolution — drop hard floor)
  - `apps/local-runner/internal/runner/supabase_catalog_store.go`, `provider_event.go` (model DTO plumbing, if extended)
  - `apps/local-runner/internal/runner/flow_definition_resolver.go`, `supabase_workflow_flow_store.go` (mirror-sync preservation)
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (mandatory model, built-in model unlock)
  - `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx` (main-agent card, greyed dropdown + tooltip)
  - `apps/desktop-flowpilot/src/state/store.ts` (Flow-mode launch — verify no chat model sent)
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` (built-in model_override write path, if Q-2 requires)
- modules: local-runner model resolution + mirror sync; desktop chat workspace + workflows settings; client-core admin repository
- routes: desktop Settings › Workflows; desktop Chat/Flow workspace
- tables: `workflows` (`model_override`, `reasoning_effort_override`, `yolo_mode`), `step_definitions` (`model`, `reasoning_effort`), `projects` (`default_model`, `default_reasoning_effort`) — read/resolve only; no schema change expected unless Q-2 needs a patch endpoint
- explicitly not touched: `apps/admin-web` (legacy), `workflow_steps` override columns (removed by BUG-164)

## 6. Acceptance Check

- Case 1 (Chat): controller-selected provider/model/reasoning is used; sub-agents spawned in chat inherit the controller model.
- Case 2 (Flow normal): the run uses `workflows.model_override`.
- Case 3 (Flow single-step): the run uses the step type's `step_definitions.model`.
- Built-in flow: Model override + Reasoning + YOLO mode are editable and persist; structural fields stay locked; a pack re-sync does not reset the user's model/reasoning/YOLO mode.
- No run ever silently uses `gpt-5.4`: with Step/Flow/Project all unset, the run is blocked and the entry is greyed with a tooltip; setting a model makes it runnable.
- Desktop: no model/provider shown until a flow is selected; then a main-agent card shows the resolved model + derived provider.
- SS-05 §2.1/§3 and SD-06 §6 are updated to describe the removed hard floor and the mandatory/greyed rule.

## 7. Out of Scope

- Adding `model:`/`provider:` to built-in pack YAML or agent markdown.
- A per-agent model override UI for chat sub-agents (rejected in favor of inherit-controller).
- Reintroducing per-`workflow_steps` overrides (removed by BUG-164).
- Per-step model re-resolution during a single multi-step run (SD-06 §6.2 known limitation).
- Any `apps/admin-web` change; provider account selection/switching and quota handling.

## 8. Completion Notes

- result: done — corrected 2026-07-06 (this doc's original "Not implemented in this turn (planning only)" note is stale; the design was substantially built in later BUG-165/171/229/235-follow-up/241 work). Verified against code: T-1 (no `gpt-5.4` hard floor; `no_model_configured` error instead) CONFIRMED via `TestCreateRunReturnsErrorWhenNoModelConfigured`/`TestCreateRunSingleStepDoesNotFallBackToProjectModel`. T-3 (unlock built-in model/reasoning/YOLO editing; Q-2's write path resolved as a scoped Supabase `update`, not a clone, that mirror-sync's `merge-duplicates` upsert never overwrites) CONFIRMED in `WorkflowsSettings.tsx`/`supabaseAdminRepository.ts`. T-4 (Step>Flow>Project tiers + BUG-171 provider reconciliation) CONFIRMED via 15 passing tests in `workflow_model_resolution_test.go`. T-5 (chat sub-agents inherit controller model) CONFIRMED via `TestSpawnChildRunInheritsParentModelDefaults`. `go build ./...` clean.
- correction 2026-07-06: an earlier verification pass wrongly claimed T-6 ("no Main agent card exists") as a gap. That's false — commit `10421b9` ("integrate model/provider directly into Agents panel and control its visibility Task-183") deliberately moved the card out of `ChatWorkspace.tsx` into `apps/desktop-flowpilot/src/components/AgentsPanel.tsx`: `hideUntilFlowTargetSelected` returns `null` from the whole panel until a flow/step is selected, then renders `mainProvider`/`mainModel` via `resolveMainAgentDisplay`. [BUG-227](../../09-BugFix/done/BUG-227-Flow-Mode-Main-Card-Shows-Pre-Run-Catalog-Model-Instead-Of-Resolved-Model.md) (done) already fixes a bug in this exact live behavior, further confirming it predates and survives this task. T-6 is CONFIRMED, not a gap.
- accepted as-is 2026-07-06 (owner decision, no further code change): T-2's `DEFAULT_MODEL = "gpt-5.4"` seeded into new workflow/step drafts and as a dropdown fallback (`WorkflowsSettings.tsx`) is kept — Save is correctly blocked when the model is empty (the functional "no silent hardcoded default at runtime" requirement, T-1, is fully met), so a pre-filled starting value in a fresh draft is a UI convenience, not a violation of user ownership. T-7's disabled unresolved-model entries use inline label text (`" (No model set)"`) rather than a hover tooltip — accepted as an equivalent, simpler UX; no `title=` tooltip added.
- upstream docs updated: SS-05/SD-06 already carry the hard-floor-removal and mandatory-model documentation (verified present); no further upstream doc change identified as required.
- verification: `go build ./...` clean; `TestCreateRunReturnsErrorWhenNoModelConfigured`, `TestCreateRunSingleStepDoesNotFallBackToProjectModel`, `TestSpawnChildRunInheritsParentModelDefaults`, and the 15 `workflow_model_resolution_test.go` cases all pass.
