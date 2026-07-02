# BUG-165: Implement Step > Flow > Project > Default Model Resolution

## Metadata

- Document ID: `BUG-165`
- Title: `Implement Step > Flow > Project > Default Model Resolution`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`, `requirements/09-BugFix/done/BUG-164-Remove-Workflow-Steps-Model-Provider-Reasoning-Overrides.md`
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, desktop, workflow-steps, execution`

## AI Quick View

### Summary

- Follow-up to `BUG-164`: the user asked whether the live desktop app was actually running steps/workflows on the correct model, given the earlier finding that the desktop client sends no `model` at all for workflow-mode runs and Go's `createRun` had no fallback (`rs.modelName = in.Model` passthrough, staying empty).
- Confirmed via `resolvePromptExecutionAdapter` (`runner.go:779-810`): an empty model doesn't error, but it does mean the underlying CLI (Codex/Claude/Gemini) falls back to *its own* default model, never FlowPilot's project/workflow/step configuration — a real, pre-existing execution gap, not something any earlier fix in this session touched (those were all display-only).
- The user confirmed the desktop chat controller (provider/model/reasoning picker) is correctly gated to `normal_chat` only (`ChatInput.tsx:737`, already existing) and must stay authoritative for direct chat (Req 1); Flow Mode has no such picker and must resolve its model itself via **Step > Flow > Project > default** (Req 2).
- This implements that resolution for real, in the Go runner, and updates `SS-05`/`SD-06` to describe the corrected architecture (post-`BUG-164`: no `workflow_steps` override, `step_definitions.model` is the "Step" tier).

### Current Ask

- Workflow/step-mode runs must resolve their model via Step (`step_definitions.model`) → Flow (`workflows.model_override`) → Project (`projects.default_model`) → hard default (`gpt-5.4`) — whichever tier has a value first, wins.
- `normal_chat` runs must keep using exactly whatever model the user explicitly selects in the chat controller, untouched by this resolution.
- Update `SS-05`/`SD-06` to reflect the corrected, current architecture.

### Key Decisions

- `F-1` `Project`/`Workflow`/`Step` Go catalog DTOs (`provider_event.go`) gain a `Model` field, sourced respectively from `projects.default_model`, `workflows.model_override`, and `step_definitions.model` (via `SupabaseCatalogStore`'s `ListProjects`/`ListWorkflows`/`ListSteps`/`ListWorkflowSteps` — each query extended to select/embed the relevant column).
- `F-2` `createRun` (`interactive_handlers.go`) resolves `resolvedModel` once, at run start, only for the `WorkflowID != ""` branch: the entry step's `Model` wins if set, else the workflow's `Model` (looked up from `ListWorkflows()`), else the project's `Model` (looked up from `ListProjects()`), else the hard floor `"gpt-5.4"`. `resolvedModel` replaces the previous raw `in.Model` passthrough for both provider derivation and `rs.modelName`.
- `F-3` `normal_chat` runs are unaffected — `resolvedModel` only gets assigned inside the `WorkflowID != ""` branch; a chat run's `in.Model` (whatever the client explicitly sent) flows through unchanged, matching Req 1.
- `F-4` No UI change was needed for "Flow Mode must not show the chat controller" (Req 2's UI half) — `ChatInput.tsx:737`'s `{isChatMode && (...)}` gate already hides the entire provider/model/reasoning controller outside `normal_chat`; verified, not modified.
- `F-5` `SS-05-Workflow-Ai-Provider.md` §1.2/§2.2/§3 and `SD-06-AI-Provider-Integration.md` §6/§7/§8 updated to describe the actual current architecture: no `workflow_steps` override (per `BUG-164`), `step_definitions.model` as the "Step" tier, resolution happening once at run start (not per-turn), and chat mode's exemption stated explicitly.

### Constraints

- This resolves the model **once, at run start**, from the workflow's entry step — it does not re-resolve per step as a multi-step workflow run advances (the classic engine's turn-starting code only ever reads a single `rs.modelName` for the run's whole lifetime; changing that is a larger structural change, explicitly flagged as a known limitation in `SD-06` §6.2 rather than silently implied as solved).
- `Reasoning Effort` resolution is unchanged (`launchOverride ?? workflow.reasoning_effort_override ?? project.default_reasoning_effort ?? "medium"`) — `step_definitions` has no reasoning-effort resolution role, matching the pre-existing SD-06 §6.2 rule; not altered by this fix.

### Open Questions

- Should genuine per-step model re-resolution during a single run's execution be built next? Not requested yet; flagged as follow-up in `SD-06` §6.2's "Known limitation" note.

### Source Refs

- `apps/local-runner/internal/runner/runner.go:779-810` (`resolvePromptExecutionAdapter` — confirms an empty model silently omits `--model`, doesn't error)
- `apps/local-runner/internal/runner/interactive_service.go:1238,2022` (`ModelName: rs.modelName` — confirms one model per run's whole lifetime, no per-step switch today)
- `apps/local-runner/internal/runner/interactive_handlers.go` (`createRun` — the new resolution)
- `apps/local-runner/internal/runner/provider_event.go` (`Project`/`Workflow`/`Step` — new `Model` fields)
- `apps/local-runner/internal/runner/supabase_catalog_store.go` (`ListProjects`/`ListWorkflows`/`ListSteps`/`ListWorkflowSteps` — extended selects)
- `apps/desktop-flowpilot/src/components/ChatInput.tsx:737` (chat-controller gate — verified pre-existing, unmodified)
- `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md` §1.2, §2.2, §3
- `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md` §6, §7, §8

## 1. Issue Summary

Before this fix, a workflow/step-mode run's model was never resolved at all — the desktop client sends no model for that mode, and the Go runner had no fallback, so the run executed on whatever the underlying CLI defaulted to, silently ignoring the user's project/workflow/step model configuration.

## 2. Parent Links

- impacted coding plan: `none`
- impacted tech design: `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md` (§6, §7, §8 updated)
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md` (§1.2, §2.2, §3 updated)

## 3. Environment and Reproduction

- environment: desktop-flowpilot, local-runner, Flow Mode / workflow-mode runs.
- reproduction steps (pre-fix): start any workflow-mode run; the run's model was never resolved from project/workflow/step configuration, only from whatever the underlying CLI itself defaulted to.
- frequency: deterministic — every workflow-mode run was affected, always.

## 4. Expected vs Actual

- expected: workflow/step-mode runs resolve Step > Flow > Project > default; chat-mode runs use exactly the explicitly-selected model.
- actual (pre-fix): workflow/step-mode runs resolved nothing; chat-mode runs already worked correctly (unaffected, confirmed not broken).

## 5. Impact

- users affected: anyone running Flow Mode / workflow-mode.
- workflows affected: model/provider selection for every workflow-mode run.
- severity: medium — not a crash, but FlowPilot's own model configuration was silently not applied to real execution for this entire run mode.

## 6. Root Cause

- confirmed cause: the desktop client never sent a model for workflow-mode `startRun` calls (no chat-controller equivalent exists for that mode), and `createRun`'s `modelName: in.Model` passthrough had no fallback logic at all.
- evidence: see Source Refs.

## 7. Fix Strategy

- `F-1`..`F-5` as described in Key Decisions.

## 8. Validation

- `V-1` `go build ./...` in `apps/local-runner` — passes.
- `V-2` `go test ./internal/runner/...` — full suite passes except the same 16 pre-existing, unrelated failures already present before this fix (Windows path mismatches, mocked Codex resume exec, skills-merge ordering, run-history ordering) — none reference `createRun`, catalog `Model` fields, or the resolution logic.
- `V-3` New tests added (`workflow_model_resolution_test.go`): `TestCreateRunResolvesModelFromStepWhenSet`, `TestCreateRunFallsBackToFlowWhenStepModelEmpty`, `TestCreateRunFallsBackToProjectWhenStepAndFlowModelEmpty`, `TestCreateRunFallsBackToHardDefaultWhenNothingConfigured`, `TestCreateRunNeverInjectsResolvedModelForChat` — all pass, driving `createRun` end-to-end through a custom fake catalog and asserting the resolved model (read directly off the in-process run state, since the default test provider registry only marks Codex selectable).
- `V-4` `TestSupabaseCatalogStoreShaping` (`partd_test.go`) updated for the new `default_model`/`model_override`/`model` select columns and asserts the new `Model` fields decode correctly.
- `V-5` `npm run typecheck` in `apps/desktop-flowpilot` — clean (no frontend code changed by this fix; confirmed `ChatInput.tsx`'s existing gate needed no edit).
- `V-6` Not executed: a live re-check against a running desktop app and real Supabase instance — no backend available in this environment (same limitation as every fix this session).

## 9. Regression Guard

- tests: the four new resolution tests plus the updated catalog-shaping test lock in the priority order and the chat-mode exemption.
- alerts: none.
- audit checks: recorded in `change-audit/CA-202-implement-step-flow-project-default-model-resolution.md`.

## 10. Follow-Up Document Updates

- upstream docs updated as part of this fix: `SS-05-Workflow-Ai-Provider.md`, `SD-06-AI-Provider-Integration.md` (see Source Refs for exact sections).
- notes left unchanged on purpose: genuine per-step model re-resolution during a single run's execution (as opposed to once at run start) remains unimplemented and is explicitly flagged as a known limitation in `SD-06` §6.2, not silently implied as solved by this fix.
