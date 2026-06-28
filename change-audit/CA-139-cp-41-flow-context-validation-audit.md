# CA-139: CP-41 Flow Context Validation Audit

## Scope

Implement the CP-41 Flow Mode runtime path: deterministic context package creation, plan-to-coding handoff, bounded testing validation retry, and audit draft/commit prep on the existing workflow run/step model.

## Completed

- Added a typed `FlowContextPackage` builder that resolves feature history, discussion history, and bounded source excerpts without vector or embedding retrieval.
- Persisted flow context packages as runner artifacts keyed by `workflow_run_id` and plan `workflow_step_run_id`.
- Wired coding-step prompt composition to reuse the latest plan package and keep the handoff stable in the logged prompt.
- Added flow-mode config loading for validation commands, retry budget, and package size defaults.
- Added bounded validation summaries, retry prompt construction, validation-result artifact storage, and retry scheduling from testing failures.
- Added audit draft generation with change-ledger block output, commit-message preparation, and audit draft artifact storage.
- Added targeted tests for package building, prompt handoff, validation summary bounding, audit draft formatting, and artifact persistence/reload.

## Verification

- `go test ./internal/runner -run 'TestBuildFlowContextPackage|TestFlowContextPackagePersistsAndReloads|TestCodingPromptIncludesFlowContextOnce|TestFlowAuditDraft|TestValidationRetryPromptIncludesOriginalContextPackage|TestSummarizeFlowValidationOutputIsBounded|TestLoadFlowModeConfigDefaultsWhenMissing'`
- `go test ./internal/runner -run 'TestLiveChatInjectsFeatureHistoryForSupportedProviders|TestLiveChatRefreshesLedgerBeforeFeatureHistoryInjection|TestLoadHandoffSummaryMatchesAcrossOffFeatureTurns|TestOrchestratorProgressSequencesAndPersists|TestPlanWorkflowProgress_CompletesNonApprovalSteps|TestPlanRejectedStepRetry'`

## Residual Notes

- The new validation retry loop is intentionally bounded and uses existing workflow step retry counts plus artifacts rather than a parallel retry session model.
- Audit draft generation still waits on the explicit flow path to write or commit; it only prepares inspectable draft state.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: CP-41
change_type: feature
summary: Implement CP-41 Flow Mode context packages, validation retries, and audit drafts
# --->8---
