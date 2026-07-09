# Task-171: Audit Step Draft And Commit Prep

## Metadata

- Document ID: `Task-171`
- Title: `Audit Step Draft And Commit Prep`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-28`
- Last Updated: `2026-07-06`
- Parent Documents: [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/todo/CP-41-RAG-Harness-Flow-Mode.md), [Task-170: Testing Feedback Retry Loop](Task-170-Testing-Feedback-Retry-Loop.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Child Documents: `None`
- Related Documents: [Task-096: Commit-History Ledger](../done/Task-096-Commit-History-Ledger.md), [Task-157: Improve Context Hardness](../done/Task-157-Improve-Context-Hardness.md), [CP-22: Audit](../../07-Coding-Plan/done/CP-22-Audit.md), [CA-132: Prompt Context Continuity And Provider Handoff](../../../change-audit/CA-132-prompt-context-continuity-and-provider-handoff.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, audit, commit-message, change-ledger`

## AI Quick View

### Summary

- Add Audit-step draft generation after Flow Mode validation passes.
- Draft includes what changed, why, source refs, validation status, residual notes, and change-ledger block values.
- Commit-message prep follows the existing `[Type][feature][layer?]` contract.
- Draft is attached to the final Audit/result `workflow_step_run_id` in the current `workflow_runs` model.
- Final writing/commit stays explicit and controlled.

### Current Ask

- Implement audit draft and commit-message preparation for the successful Flow Mode path.

### Key Decisions

- `T-1` Audit output is a draft package unless existing workflow policy explicitly allows writing.
- `T-2` Draft must include `feature_key`, source doc id, validation result, changed files, and residual notes.
- `T-3` Commit message suggestion must use a registry-verified feature key.
- `T-4` Failed or skipped validation cannot produce a success audit draft.
- `T-5` Audit draft is persisted as an inspectable run artifact/event for the Audit/result step.

### Constraints

- Depends on `Task-170`.
- Must follow `SS-13` change-ledger rules.
- Must not fabricate validation success.
- Must not create commits without explicit workflow/user confirmation.
- Must not create a separate audit session outside the current run/step/artifact model.

### Open Questions

- None. Use the defaults in this task.

### Source Refs

- `CP-41 P-6`, `DOD-4`
- `SS-13` change-ledger block
- `Task-096`, `Task-157`
- current code: `artifacts.go`, `workflow_store.go`, `supabase_workflow_store.go`, `provider_event.go`

## 1. Goal

Prepare an audit-ready result at the end of a successful Flow Mode run so the future implementer gets a consistent CA-note draft and commit-message suggestion without manually reconstructing history from the run.

The result belongs to the existing flow run. Store or expose the draft under the Audit/result step's `workflow_step_run_id`, using existing artifact/event paths where possible.

## 2. Parent Links

- coding plan: `CP-41`
- tech design: `SD-17`
- system spec: `SS-13`
- specific upstream ids: `CP-41 P-6`, `CP-41 DOD-4`

## 3. Trigger

After Coding and Testing succeed, Flow Mode should close the loop by preparing the same audit material the project already expects: what changed, why, validation proof, and a feature-keyed commit message.

## 4. Exact Change

- `T-1` Add audit draft model.
  - Suggested type: `FlowAuditDraft`.
  - Required fields:
    - `FeatureKey`
    - `SourceDocID`
    - `ChangeType`
    - `Summary`
    - `WhatChanged`
    - `WhyChanged`
    - `ChangedFiles`
    - `ValidationCommands`
    - `ValidationResult`
    - `ResidualNotes`
    - `CommitMessage`
    - `ChangeLedgerBlock`

- `T-2` Build draft from Flow state.
  - Use Plan context package refs from `Task-168`.
  - Use Coding result/changed files from the run.
  - Use Testing status from `Task-170`.
  - Use feature key from the original Plan package, not from free-form final text.
  - Use `workflow_run_id`, Plan step id, Coding step id, Testing step id, and Audit/result step id for traceability.

- `T-3` Generate commit-message suggestion.
  - Format: `[Type][feature][layer?] <description> <source-doc-id>`.
  - Feature must exist in `change-audit/FEATURE-KEYS.md`.
  - If feature key is missing/unverified, draft status is `blocked_missing_feature_key`.
  - Keep first line within the existing commit-format constraints.

- `T-4` Generate CA note draft content.
  - Include concise `What`, `Why`, `Validation`, and `Residual Notes`.
  - Include a valid `flowpilot:change-ledger` block:
    - `feature_key`
    - `source_doc_id`
    - `change_type`
    - `summary`
  - Do not write the final file unless existing workflow policy allows it.

- `T-5` Surface audit draft.
  - Emit an event or make the draft available in run state/artifacts.
  - Event/artifact payload must include `workflow_run_id` and Audit/result `workflow_step_run_id`.
  - The user should be able to inspect the draft before committing.
  - Failed validation surfaces a failure summary instead of a success draft.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/workflow_orchestrator.go`
  - `apps/local-runner/internal/runner/runner.go`
  - `apps/local-runner/internal/runner/artifacts.go`
  - `apps/local-runner/internal/runner/gate_hook.go`
  - `apps/local-runner/internal/skillpack/flow-pack/common/git-commit-format/SKILL.md`
  - `apps/local-runner/internal/skillpack/flow-pack/common/audit-logging/SKILL.md`
  - optional new file: `apps/local-runner/internal/runner/flow_audit_draft.go`
- modules:
  - workflow orchestration
  - artifacts/run state
  - audit logging
  - commit-message preparation
- routes:
  - existing run artifacts/event routes; add a route only if no current artifact path can expose the draft
- tables:
  - existing: `workflow_runs`, `workflow_run_steps`, `workflow_provider_events`
  - preferred payload surface: existing artifact storage for CA draft and commit-message suggestion
  - no schema change expected

## 6. Acceptance Check

- Successful Flow Mode validation produces an audit draft with feature key, source doc id, validation result, changed files, and residual notes.
- Draft includes a valid change-ledger block.
- Commit-message suggestion uses a known feature key and includes the source doc id.
- Failed/skipped validation does not produce a success audit draft.
- Draft is inspectable before any file write or commit.
- Draft lookup is scoped to the current `workflow_run_id` and Audit/result `workflow_step_run_id`.

### 6.1 Test Items

- `TestFlowAuditDraftFromSuccessfulRun`
- `TestFlowAuditDraftIncludesChangeLedgerBlock`
- `TestFlowAuditDraftCommitMessageUsesKnownFeatureKey`
- `TestFlowAuditDraftBlocksMissingFeatureKey`
- `TestFlowAuditDraftDoesNotClaimSuccessWhenValidationFailed`
- `TestFlowAuditDraftIncludesResidualNotes`
- `TestFlowAuditDraftDoesNotWriteWithoutApproval`
- `TestFlowAuditDraftPersistsAgainstAuditStep`
- `TestFlowAuditDraftCarriesAllStepIDs`
- `TestFlowAuditDraftScopedToWorkflowRun`

### 6.2 Definition of Done

- [x] `DOD-1` `FlowAuditDraft` model exists and is covered by tests.
- [x] `DOD-2` Successful Flow Mode run can produce a draft from stored run state.
- [x] `DOD-3` Draft includes a valid `flowpilot:change-ledger` block.
- [x] `DOD-4` Commit-message suggestion follows the existing feature-key contract.
- [x] `DOD-5` Failed/skipped validation cannot produce a success draft.
- [x] `DOD-6` Draft is inspectable before any write/commit.
- [x] `DOD-7` Targeted runner/skillpack tests pass.
- [x] `DOD-8` Draft is attached to existing workflow run/step persistence and does not create a parallel audit session.

## 7. Out of Scope

- Building Plan context package. That is `Task-168`.
- Plan-to-Coding handoff. That is `Task-169`.
- Testing retry loop. That is `Task-170`.
- Automatically pushing commits or opening PRs.

## 8. Completion Notes

- result: `done` — verified 2026-07-06: all 10 test items in §6.1 exist and pass (`go test ./internal/runner/ -run 'TestFlowAuditDraft...'`), plus `go test ./internal/skillpack/...` confirms no regression. Doc's Status/DoD had never been updated to reflect the shipped implementation; corrected here. All four CP-41 child tasks (168-171) are now verified done — see `CP-41`'s own DoD, which was already correct (7/8 checked; the one remaining item, `DOD-7`, is a manual E2E run the maintainer has not yet performed themselves, so `CP-41` itself stays in `inprogress/` by design).
- follow-ups: none — `CP-41`'s own manual-verification `DOD-7` is the only remaining item, and it's tracked on `CP-41` itself, not here.
- upstream docs updated: none required; `CP-41`'s own DoD already reflected this task as complete.
- **CORRECTION 2026-07-06, prior to a manual CP-41 E2E test session: this task's own unit-level "done" verification above does NOT mean this code is reachable in a live Flow Mode run, and DoD-6 ("Draft is inspectable before any write/commit") is NOT actually satisfiable today.** Tracing the full RAG Harness pipeline found `BuildAuditDraft` has no production caller — the registered `artifact.audit_draft` behavior handler (`behaviorArtifactAuditDraft`, `behavior_registry_builtin.go:209-219`) is a stub that echoes `RawArgs["summary"]` back, never calling the real draft builder. Separately, no desktop UI surface exists anywhere to display an audit draft even if one were produced. See [BUG-243](../../09-BugFix/done/BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md) for the full trace and fix plan. This task's own DoD/tests remain accurate for what they claim (the draft builder itself is correct and unit-tested); the disconnect is in the behavior-registry wiring and missing UI, not in this task's own code.

**RESOLVED 2026-07-09**: BUG-243 is fixed — `behaviorArtifactAuditDraft`'s live dispatch path (via the new `runAuditNode` in `flow_validate_audit_dispatch.go`) now calls the real `BuildAuditDraft`/`PersistAuditDraft`, reachable via `tryAdvanceFlowFromNode`'s new mid-flow inline-dispatch path (`F-0`). DoD-6 ("inspectable before any write/commit") is now genuinely satisfiable: a desktop UI surface exists (`timelineReducer.ts`'s `flow_audit_draft` case, `F-3`) rendering a `ready`/`blocked_*` card distinctly, before any write.
