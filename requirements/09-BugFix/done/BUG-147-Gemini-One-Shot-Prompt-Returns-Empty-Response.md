# BUG-147: Gemini One-Shot Prompt Returns Empty Response

## Metadata

- Document ID: `BUG-147`
- Title: `Gemini One-Shot Prompt Returns Empty Response`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-29`
- Last Updated: `2026-06-29`
- Parent Documents: [Task-167: Gemini Resume Handoff And Live DOD](../../08-Task/inprogress/Task-167-Gemini-Resume-Handoff-And-Live-DOD.md), [CP-40: Gemini Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-40-Gemini-Adapter-Plan.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `None`
- Related Documents: [CA-140: Gemini AGY Runtime Fallback](../../../change-audit/CA-140-gemini-agy-runtime-fallback.md), [CA-141: Gemini History Replay Full Response](../../../change-audit/CA-141-gemini-history-replay-full-response.md), [CA-142: Gemini One-Shot Prompt No New Project](../../../change-audit/CA-142-gemini-one-shot-prompt-no-new-project.md)
- Replaces: `None`
- Tags: `gemini, ai-providers, local-runner, agy, regression`

## AI Quick View

### Summary

- Gemini one-shot prompt execution could return no response even after the workspace AGY project config was bootstrapped.
- The session send path already avoided `--new-project` because AGY print mode can emit no LLM response when that flag is used with an existing config.
- `resolvePromptExecutionAdapter` still passed `createProject=true` for first Gemini one-shot launches, reintroducing `--new-project`.
- The fix aligns one-shot prompt execution with the working session path and adds regression coverage.

### Current Ask

- Restore Gemini response output for one-shot prompt execution paths such as `Runner.ExecutePrompt` and Gemini chat summarization.

### Key Decisions

- `V-1` After `ensureGeminiProjectConfig` returns a project id, launch AGY with `--project <uuid>` and without `--new-project`.
- `V-2` Keep `--print <prompt>` at the end of Gemini one-shot invocations so the prompt is not confused with another flag value.

### Constraints

- Do not regress Codex or Claude branches in `resolvePromptExecutionAdapter`.
- Do not change the Gemini session send path, which already uses the corrected argument contract.
- Live Gemini validation still depends on a connected local AGY/Gemini account.

### Open Questions

- None for the code fix; live AGY behavior should still be validated on an authenticated machine.

### Source Refs

- User report: "now with gemini, can not return reponse"
- `apps/local-runner/internal/runner/runner.go`
- `apps/local-runner/internal/runner/sessions.go`
- `apps/local-runner/internal/runner/runner_test.go`
- `go test ./internal/runner -run 'TestResolvePromptExecutionAdapter|TestGeminiAdapter|TestStartSessionGemini|TestSendMessageGemini|TestCreateRunGeminiRequiresUsableWorkspacePath|TestSendTurnWithRetryDoesNotRetryGeminiWorkspaceRequired' -count=1`
- `go test ./internal/runner -count=1` failed on pre-existing unrelated tests: `TestProviderRegistryForUsesFakeWhenFlagOff`, `TestFinalizerHookSurfacesArtifacts`, three skill merge precedence tests, and `TestStartInteractiveAuthLaunchesFromWorkspace`

## 1. Issue Summary

Gemini could fail to return visible output for one-shot prompt execution. The affected path is used by `Runner.ExecutePrompt` and Gemini summary generation, separate from the interactive Gemini session send path.

## 2. Parent Links

- impacted coding plan: `CP-40`
- impacted tech design: `SD-12`, `SD-06`
- impacted system spec: `SS-11`

## 3. Environment and Reproduction

- environment: local runner, Gemini provider through AGY print mode
- reproduction steps: run a Gemini one-shot prompt through `Runner.ExecutePrompt` or the Gemini summarizer on a workspace without a previous mapped session
- frequency: expected whenever that path bootstraps a workspace project and launches AGY with `--new-project`

## 4. Expected vs Actual

- expected: AGY receives the prompt and writes the model response to stdout, which FlowPilot captures as `OutputMarkdown`
- actual: AGY could complete without a usable LLM response on stdout

## 5. Impact

- users affected: users selecting Gemini for one-shot prompt execution, summary generation, or any workflow using `Runner.ExecutePrompt`
- workflows affected: Gemini summary generation, provider-driven MCP prompt execution, and task-style one-shot prompt runs
- severity: `high` for Gemini-only workflows because the model appears unable to return a response

## 6. Root Cause

- hypothesis: AGY print mode returns empty stdout when `--new-project` is passed after FlowPilot has already bootstrapped an AGY project config
- confirmed cause: `resolvePromptExecutionAdapter` called `geminiCLIArgs(..., createProject=!resume)`, so the first one-shot Gemini prompt still included `--new-project`; the session path already documented and avoided this flag because `--project <uuid>` is sufficient once the config exists
- evidence: code inspection showed the divergent argument contract between `runner.go` and `sessions.go`; the new regression test verifies the one-shot path omits `--new-project` after project config bootstrap

## 7. Fix Strategy

- `F-1` Change `resolvePromptExecutionAdapter` to pass `createProject=false` for Gemini after resolving or bootstrapping the workspace project id.
- `F-2` Add a regression test proving Gemini one-shot prompt args include `--project` but not `--new-project` or `--continue` for a newly bootstrapped workspace config.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'TestResolvePromptExecutionAdapter|TestGeminiAdapter|TestStartSessionGemini|TestSendMessageGemini|TestCreateRunGeminiRequiresUsableWorkspacePath|TestSendTurnWithRetryDoesNotRetryGeminiWorkspaceRequired' -count=1`
- `V-2` `go test ./internal/runner -count=1` was run and failed on unrelated existing tests outside this Gemini one-shot fix.
- `V-3` Live AGY/Gemini account validation was not run in this turn.

## 9. Regression Guard

- tests: `TestResolvePromptExecutionAdapterGeminiOmitsNewProjectAfterConfigBootstrap`
- alerts: existing `[gemini-agy]` launch/result diagnostics show project id, sanitized args, stdout bytes, stderr, and exit status
- audit checks: `git diff` confirms the production change is scoped to Gemini argument construction in `resolvePromptExecutionAdapter`

## 10. Follow-Up Document Updates

- upstream docs that must change: none; this restores the expected CP-40/Task-167 Gemini response behavior without changing business intent
- notes left unchanged on purpose: existing AGY runtime fallback notes remain historical context; this bug records the later correction for one-shot prompt execution
