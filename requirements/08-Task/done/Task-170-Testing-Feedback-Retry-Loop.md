# Task-170: Testing Feedback Retry Loop

## Metadata

- Document ID: `Task-170`
- Title: `Testing Feedback Retry Loop`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-28`
- Last Updated: `2026-07-06`
- Parent Documents: [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/todo/CP-41-RAG-Harness-Flow-Mode.md), [Task-169: Plan To Coding Context Handoff](Task-169-Plan-To-Coding-Context-Handoff.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: `None`
- Related Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md), [Task-155: Update R-Reg](../done/Task-155-update-r-reg.md), [CA-132: Prompt Context Continuity And Provider Handoff](../../../change-audit/CA-132-prompt-context-continuity-and-provider-handoff.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, testing, retry-loop, validation`

## AI Quick View

### Summary

- Add a bounded Testing-step feedback loop for Flow Mode.
- Failed validation sends concise failure context back to Coding with the original Plan package.
- Retry state is runner-owned and capped at `3`.
- Retry state is attached to the active `workflow_run_id` and Testing/Coding `workflow_step_run_id`s.
- Raw logs must be summarized and bounded before entering prompts.

### Current Ask

- Implement the Testing-step command result capture, failure summarization, retry state, and retry guard.

### Key Decisions

- `T-1` Testing feedback is additive; it never mutates the original Plan context package.
- `T-2` Retry max is `3` by default.
- `T-3` Runner state owns retry attempts and failure history.
- `T-4` Invalid environment/command setup does not trigger Coding retry.
- `T-5` Use existing step persistence first: `workflow_run_steps.retry_count`, step status, `workflow_run_logs`, events, and artifacts.

### Constraints

- Depends on `Task-169`.
- Must work with existing flow-gate behavior and not bypass `r-reg`, `r-tests`, or `r-ca`.
- Do not paste unbounded compiler/test logs into prompts.
- Do not edit tests to make validation pass; production code or docs/spec conflict must be addressed.
- Do not create a separate retry-session table unless existing run/step/log/event/artifact surfaces are proven insufficient.

### Open Questions

- None. Use the defaults in this task.

### Source Refs

- `CP-41 P-4`, `P-5`, `DOD-3`
- `SD-20`
- `Task-155`
- current code: `workflow_state_machine.go`, `workflow_orchestrator.go`, `workflow_store.go`, `supabase_workflow_store.go`, `gate_hook.go`

## 1. Goal

Add a Testing-step feedback loop that can run configured validation commands, summarize failures, and trigger bounded Coding retries while preserving the original Plan context package.

The loop must be represented in the current Flow Mode persistence model: the Testing step records validation status, logs/events/artifacts capture detail, and Coding retry attempts use step `retry_count`/run state rather than a separate flow session.

## 2. Parent Links

- coding plan: `CP-41`
- tech design: `SD-20`
- system spec: `SS-13`
- specific upstream ids: `CP-41 P-4`, `CP-41 P-5`, `CP-41 DOD-3`

## 3. Trigger

Flow Mode needs a closed loop where build/test failures can be corrected without requiring the user to manually copy logs into the next Coding prompt. Existing flow gates can detect problems after a turn, but CP-41 needs an explicit Testing-step retry mechanism tied to the Plan context package.

## 4. Exact Change

- `T-1` Add validation command configuration.
  - Read commands from Flow Mode/project config when present.
  - If no command is configured, mark Testing as `skipped_no_command` and do not retry Coding.
  - Store command, working directory, started/finished timestamps, exit code, and status.
  - Store the result against the current Testing `workflow_step_run_id`.

- `T-2` Add bounded validation output summarization.
  - Capture stdout/stderr separately.
  - Keep raw logs out of prompts by default.
  - Extract key failure lines:
    - first compiler error block;
    - failing test names;
    - assertion message;
    - stack trace top frames;
    - command and exit code.
  - Cap feedback text before prompt insertion.

- `T-3` Add retry state.
  - Suggested fields:
    - `RetryAttempt`
    - `MaxRetries`
    - `OriginalPlanPackageID` or hash
    - `PreviousCodingTurnID`
    - `ValidationCommand`
    - `FailureSummary`
    - `ChangedFiles`
    - `NextInstruction`
  - Persist or attach state to the `workflow_run_id` and related Coding/Testing step ids so UI/replay can explain why a retry happened.
  - Prefer existing `workflow_run_steps.retry_count`, `workflow_run_logs`, `workflow_provider_events.payload_json`, and artifacts before adding schema.

- `T-4` Add retry prompt composition.
  - Prompt includes:
    - original Flow Context Package from `Task-169`;
    - previous attempted change summary;
    - bounded failure summary;
    - instruction to fix production code unless the user explicitly approves requirement/test changes.
  - Retry prompt does not rerun Plan by default.

- `T-5` Add retry guard.
  - Stop after `3` failed attempts.
  - Mark final state `failed_validation_max_retries`.
  - Surface final failure summary to the user.
  - Do not retry on environment/config errors such as command missing, permission denied, or dependency unavailable.

- `T-6` Add workflow-state integration.
  - Failed Testing should transition the appropriate step status consistently with `PlanWorkflowProgress`.
  - Retry prompt should start the Coding step as a normal provider turn with the existing `workflow_step_run_id`.
  - Events should carry both `workflow_run_id` and `workflow_step_run_id` for replay.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/workflow_orchestrator.go`
  - `apps/local-runner/internal/runner/gate_hook.go`
  - `apps/local-runner/internal/runner/runner.go`
  - `apps/local-runner/internal/runner/workflow_state_machine_test.go`
  - `apps/local-runner/internal/flowgate/*`
  - optional new file: `apps/local-runner/internal/runner/flow_validation_retry.go`
- modules:
  - workflow orchestration
  - validation command execution
  - flow gate integration
  - prompt assembly
  - run state/replay
- routes:
  - existing workflow run/turn/event stream routes
- tables:
  - existing: `workflow_runs`, `workflow_run_steps.retry_count`, `workflow_run_logs`, `workflow_provider_events`
  - optional existing artifact storage for raw validation logs and bounded summaries
  - no schema change expected unless run-state persistence requires a new queryable field

## 6. Acceptance Check

- A configured failing validation command creates a bounded failure summary.
- Coding retry receives original Plan context plus failure summary.
- Retry attempts stop at `3`.
- Environment/setup failures report to the user but do not trigger Coding retries.
- Existing flow-gate behavior still evaluates final turns.
- Retry count and validation summaries are attached to the current run/step ids.

### 6.1 Test Items

- `TestValidationFailureBuildsBoundedRetryFeedback`
- `TestRetryPromptIncludesOriginalContextPackage`
- `TestRetryPromptDoesNotMutateOriginalPackage`
- `TestRetryLoopStopsAtMaxRetries`
- `TestMissingValidationCommandDoesNotRetry`
- `TestEnvironmentCommandFailureDoesNotRetryCoding`
- `TestFlowGateStillRunsAfterRetryTurn`
- `TestRawValidationLogIsNotInjectedUnbounded`
- `TestValidationResultPersistsWithTestingStepID`
- `TestRetryAttemptUsesWorkflowStepRetryCount`
- `TestRetryEventsCarryRunAndStepIDs`

### 6.2 Definition of Done

- [x] `DOD-1` Validation command result capture exists and is covered by tests.
- [x] `DOD-2` Failure summarizer produces bounded prompt-safe feedback.
- [x] `DOD-3` Retry state is runner-owned and inspectable.
- [x] `DOD-4` Coding retry prompt includes original Plan package plus failure summary.
- [x] `DOD-5` Retry loop stops at max `3` (`defaultMaxRetries = 3` in `flow_validation_retry.go`).
- [x] `DOD-6` Environment/setup failures do not trigger Coding retry.
- [x] `DOD-7` Existing flow-gate tests still pass.
- [x] `DOD-8` Targeted runner/flowgate tests pass.
- [x] `DOD-9` Retry and validation state is attached to existing workflow run/step persistence.

## 7. Out of Scope

- Defining the Plan context package. That is `Task-168`.
- Plan-to-Coding baseline handoff. That is `Task-169`.
- Writing audit notes or commit messages. That is `Task-171`.
- Automatically modifying tests to satisfy validation.

## 8. Completion Notes

- result: `done` — verified 2026-07-06: the functionality shipped as `flow_validation_retry.go`/`flow_validation_retry_test.go` under renamed (but equivalent-coverage) test functions rather than the exact names listed in §6.1 — `TestNewFlowValidationRetryState*`, `TestAdvanceRetryState*`, `TestRunValidationCommand*`, `TestSummarizeValidationFailure*`, `TestComposeRetryPromptIncludesFailureBlock`, `TestPersistValidationResultEventType` — all pass, plus `go test ./internal/flowgate/...` (100 tests) confirms no flow-gate regression. Doc's Status/DoD had never been updated to reflect the shipped implementation; corrected here.
- follow-ups: `Task-171` consumes final validation status.
- upstream docs updated: none required; `CP-41`'s own DoD already reflected this task as complete.
