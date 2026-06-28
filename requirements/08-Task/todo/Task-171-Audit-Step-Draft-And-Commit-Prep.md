# Task-171: Audit Step Draft And Commit Prep

## Metadata

- Document ID: `Task-171`
- Title: `Audit Step Draft And Commit Prep`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-28`
- Last Updated: `2026-06-28`
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
- Final writing/commit stays explicit and controlled.

### Current Ask

- Implement audit draft and commit-message preparation for the successful Flow Mode path.

### Key Decisions

- `T-1` Audit output is a draft package unless existing workflow policy explicitly allows writing.
- `T-2` Draft must include `feature_key`, source doc id, validation result, changed files, and residual notes.
- `T-3` Commit message suggestion must use a registry-verified feature key.
- `T-4` Failed or skipped validation cannot produce a success audit draft.

### Constraints

- Depends on `Task-170`.
- Must follow `SS-13` change-ledger rules.
- Must not fabricate validation success.
- Must not create commits without explicit workflow/user confirmation.

### Open Questions

- None. Use the defaults in this task.

### Source Refs

- `CP-41 P-6`, `DOD-4`
- `SS-13` change-ledger block
- `Task-096`, `Task-157`

## 1. Goal

Prepare an audit-ready result at the end of a successful Flow Mode run so the future implementer gets a consistent CA-note draft and commit-message suggestion without manually reconstructing history from the run.

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
  - no schema change expected

## 6. Acceptance Check

- Successful Flow Mode validation produces an audit draft with feature key, source doc id, validation result, changed files, and residual notes.
- Draft includes a valid change-ledger block.
- Commit-message suggestion uses a known feature key and includes the source doc id.
- Failed/skipped validation does not produce a success audit draft.
- Draft is inspectable before any file write or commit.

### 6.1 Test Items

- `TestFlowAuditDraftFromSuccessfulRun`
- `TestFlowAuditDraftIncludesChangeLedgerBlock`
- `TestFlowAuditDraftCommitMessageUsesKnownFeatureKey`
- `TestFlowAuditDraftBlocksMissingFeatureKey`
- `TestFlowAuditDraftDoesNotClaimSuccessWhenValidationFailed`
- `TestFlowAuditDraftIncludesResidualNotes`
- `TestFlowAuditDraftDoesNotWriteWithoutApproval`

### 6.2 Definition of Done

- [ ] `DOD-1` `FlowAuditDraft` model exists and is covered by tests.
- [ ] `DOD-2` Successful Flow Mode run can produce a draft from stored run state.
- [ ] `DOD-3` Draft includes a valid `flowpilot:change-ledger` block.
- [ ] `DOD-4` Commit-message suggestion follows the existing feature-key contract.
- [ ] `DOD-5` Failed/skipped validation cannot produce a success draft.
- [ ] `DOD-6` Draft is inspectable before any write/commit.
- [ ] `DOD-7` Targeted runner/skillpack tests pass.

## 7. Out of Scope

- Building Plan context package. That is `Task-168`.
- Plan-to-Coding handoff. That is `Task-169`.
- Testing retry loop. That is `Task-170`.
- Automatically pushing commits or opening PRs.

## 8. Completion Notes

- result: `pending`
- follow-ups: final CP-41 rollout can mark done after all child tasks pass.
- upstream docs updated: update `CP-41` only if audit ownership or write policy changes.
