# Task-168: Flow Mode Context Package Contract

## Metadata

- Document ID: `Task-168`
- Title: `Flow Mode Context Package Contract`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-28`
- Last Updated: `2026-06-28`
- Parent Documents: [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/todo/CP-41-RAG-Harness-Flow-Mode.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Child Documents: `None`
- Related Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-37: Prompt Context Continuity](../../07-Coding-Plan/done/CP-37-Prompt-Context-Continuity.md), [Task-157: Improve Context Hardness](../done/Task-157-Improve-Context-Hardness.md), [Task-161: Per-Feature Chat-Summary Timeline](../done/Task-161-Per-Feature-Chat-Summary-Timeline.md), [Task-163: Chat-Summary Generation Triggers](../done/Task-163-Chat-Summary-Generation-Triggers.md), [CA-132: Prompt Context Continuity And Provider Handoff](../../../change-audit/CA-132-prompt-context-continuity-and-provider-handoff.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, context-package, deterministic-retrieval, no-vector`

## AI Quick View

### Summary

- Define the typed/bounded Flow Mode context package created by the Plan step.
- Reuse existing deterministic feature-history and chat-summary seams instead of building a parallel retriever.
- Include source references, confidence status, and bounded excerpts for downstream steps.
- Bind the package to the existing `workflow_run_id` and Plan `workflow_step_run_id`.
- Explicitly exclude vector DB, embeddings, and similarity search.

### Current Ask

- Build the implementation-ready contract and package builder for Plan-step context assembly.

### Key Decisions

- `T-1` The context package is a structured runner-owned artifact, not free-form prompt text.
- `T-2` Retrieval sources are direct lookups only: `feature_key`, feature catalog, CA/commit ledger, chat-summary ledger, explicit source files, and upstream docs.
- `T-3` Every included excerpt must carry a source reference.
- `T-4` Missing history is a valid state and must degrade to source refs/confidence notes, not an error.
- `T-5` No vector DB, embedding index, or similarity search is allowed.
- `T-6` The package belongs to the current Flow Mode run and Plan step; it is not a standalone session.

### Constraints

- Extend `apps/local-runner/internal/runner/feature_history.go` and existing `featurecatalog` slots where possible.
- Do not duplicate `HistorySlot` or `ChatSummarySlot` behavior under a new hidden retriever.
- Keep package content bounded; prefer references over full text when size limits are reached.
- Preserve current chat-mode prompt-context behavior while adding Flow Mode package support.
- Use existing workflow persistence boundaries: `WorkflowStore`, step logs/events/artifacts, and Supabase only through store abstractions.

### Open Questions

- None. Use the defaults in this task.

### Source Refs

- `CP-41 P-1`, `P-2`, `DOD-1`, `DOD-5`
- `SD-17 D-4`
- `Task-157`, `Task-161`, `Task-163`
- `CA-132`
- current code: `workflow_store.go`, `supabase_workflow_store.go`, `workflow_orchestrator.go`, `workflow_state_machine.go`, `feature_history.go`

## 1. Goal

Create a deterministic `FlowContextPackage` contract and builder that the Plan step can use to assemble all relevant context for a Flow Mode issue/bug run.

The package must be complete enough for later steps to consume without broad re-retrieval and precise enough for tests to prove no vector/embedding path is involved.

The package identity must include the current `workflow_run_id` and Plan `workflow_step_run_id` because Flow Mode already persists a run as one row with N ordered step rows. Future steps must look up the package through the current run/step relationship.

## 2. Parent Links

- coding plan: `CP-41`
- tech design: `SD-17`, `SD-20`
- system spec: `SS-13`
- specific upstream ids: `CP-41 P-1`, `CP-41 P-2`, `CP-41 DOD-1`, `CP-41 DOD-5`

## 3. Trigger

CP-41 needs a stable Plan-step output before Coding, Testing, and Audit can be wired. The existing `injectFeatureHistoryPrompt` path already injects history into prompts, but Flow Mode needs a named package with sections, source refs, confidence, and bounded source excerpts.

## 4. Exact Change

- `T-1` Add a context package model in the runner layer.
  - Suggested types:
    - `FlowContextPackage`
    - `FlowContextSourceRef`
    - `FlowContextSection`
    - `FlowContextConfidence`
  - Required fields:
    - `WorkflowRunID`
    - `PlanStepRunID`
    - `PackageID` or deterministic package hash
    - `FeatureKey`
    - `FeatureConfidence`
    - `SourceDocIDs`
    - `HistoryBlock`
    - `DiscussionBlock`
    - `SourceExcerpts`
    - `Constraints`
    - `Warnings`
    - `Omitted`

- `T-2` Add a package builder.
  - Suggested function: `BuildFlowContextPackage(workspace, prompt string, priorTurns []transcriptTurn, hints FlowContextHints) (FlowContextPackage, error)`.
  - `FlowContextHints` must include `workflow_run_id`, Plan `workflow_step_run_id`, user prompt, source doc id, changed paths, and optional explicit source paths.
  - Resolve feature through existing `resolveInjectionFeature` behavior.
  - Load prior work through `composeFeatureBlocks` or lower-level `HistorySlot`/`ChatSummarySlot`.
  - Do not call any vector, embedding, or similarity-search dependency.

- `T-3` Add explicit source-file excerpt support.
  - Accept paths from stack traces, changed paths, feature catalog file globs, or user-provided paths.
  - Only read files under the workspace.
  - Cap each excerpt and total excerpt bytes.
  - Preserve omitted-file reasons such as `outside_workspace`, `too_large`, `not_found`, or `binary`.

- `T-4` Add deterministic packing rules.
  - Priority order:
    1. source refs and confidence status
    2. newest prior truth from CA/commit history
    3. explicit issue/user ask
    4. source excerpts
    5. prior discussion summaries
    6. warnings and omitted refs
  - When content is omitted, keep the source ref and reason.

- `T-5` Add package rendering for later tasks.
  - Suggested function: `RenderFlowContextPackage(pkg FlowContextPackage) string`.
  - Render one stable Markdown block headed `## Flow Context Package`.
  - Include a `No vector retrieval used` line for audit/debug visibility.

- `T-6` Add package persistence path.
  - Store the package as an artifact or provider/workflow event associated with the Plan `workflow_step_run_id`.
  - The package may also be cached in memory for the live run, but durable lookup must use existing run/step ids.
  - Do not add a new table by default. Add a schema migration only if tests prove artifacts/events/logs cannot support the required lookup.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/feature_history.go`
  - `apps/local-runner/internal/runner/feature_history_test.go` or existing runner prompt tests
  - `apps/local-runner/internal/featurecatalog/slots.go`
  - `apps/local-runner/internal/featurecatalog/chat_summary_slots.go`
  - optional new file: `apps/local-runner/internal/runner/flow_context_package.go`
  - optional new test file: `apps/local-runner/internal/runner/flow_context_package_test.go`
- modules:
  - runner prompt/context assembly
  - feature catalog
  - change ledger and chat-summary ledger
- routes:
  - none
- tables:
  - existing: `workflow_runs`, `workflow_run_steps`, `workflow_run_logs`, `workflow_provider_events`
  - optional: existing artifact storage for large package payloads

## 6. Acceptance Check

- A verified feature key produces a package with feature history, CA excerpts, chat-summary block when present, and source refs.
- An ambiguous or missing feature key produces warnings and no wrong-history injection.
- Missing ledger files degrade to a package with warnings and available source refs.
- Source excerpts are bounded and only read from workspace-safe paths.
- The rendered package is stable Markdown and includes `No vector retrieval used`.
- The package can be persisted and looked up by `workflow_run_id` + Plan `workflow_step_run_id`.

### 6.1 Test Items

- `TestBuildFlowContextPackageVerifiedFeatureIncludesHistory`
- `TestBuildFlowContextPackageIncludesChatSummaryWhenPresent`
- `TestBuildFlowContextPackageLowConfidenceDoesNotInjectWrongHistory`
- `TestBuildFlowContextPackageMissingLedgersDegrades`
- `TestBuildFlowContextPackageSourceExcerptCapsAndOmissions`
- `TestBuildFlowContextPackageRejectsOutsideWorkspacePath`
- `TestBuildFlowContextPackageNoVectorDependency`
- `TestRenderFlowContextPackageStableSections`
- `TestFlowContextPackageCarriesRunAndStepIDs`
- `TestFlowContextPackagePersistsAgainstPlanStep`
- `TestFlowContextPackageLookupSurvivesRunnerRestart`
- `TestFlowContextPackageDoesNotCreateParallelSessionState`

### 6.2 Definition of Done

- [ ] `DOD-1` `FlowContextPackage` and source-ref models exist and are documented by tests.
- [ ] `DOD-2` Package builder uses existing feature resolver/history/chat-summary seams.
- [ ] `DOD-3` Package builder has no vector DB, embedding, or similarity-search dependency.
- [ ] `DOD-4` Source excerpts are workspace-safe, bounded, and source-referenced.
- [ ] `DOD-5` Missing/ambiguous context degrades with warnings instead of failing the Flow Mode run.
- [ ] `DOD-6` Renderer emits stable Markdown for `Task-169`.
- [ ] `DOD-7` Targeted Go tests pass for runner/featurecatalog/changeledger packages touched by this task.
- [ ] `DOD-8` Package persistence is attached to existing run/step ids and does not create a parallel flow/session store.

## 7. Out of Scope

- Passing the package to Coding. That is `Task-169`.
- Running validation commands or retries. That is `Task-170`.
- Audit draft generation. That is `Task-171`.
- Any vector DB, embedding, or semantic similarity retrieval.

## 8. Completion Notes

- result: `pending`
- follow-ups: `Task-169` consumes the renderer output.
- upstream docs updated: update `CP-41` only if implementation changes the task boundaries.
