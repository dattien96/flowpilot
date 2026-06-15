# BUG-055: Claude Session Execution Bypasses Usage Limit Normalization

## Metadata

- Document ID: `BUG-055`
- Title: `Claude session execution bypasses usage limit normalization`
- Phase: `bugfix`
- Status: `done`
- Owner: `Codex`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`, `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- Child Documents: ``
- Related Documents: `requirements/09-BugFix/done/BUG-051-Claude-Limit-Reported-As-Login.md`, `requirements/09-BugFix/done/BUG-054-Claude-Usage-Limit-Message-Contains-Confusing-Login-Phrase.md`, `change-audit/CA-068-fix-claude-session-usage-limit-normalization.md`
- Replaces: ``
- Tags: `desktop`, `local-runner`, `claude`, `provider-runtime`, `chat`, `regression`

## AI Quick View

### Summary

- The Claude workflow adapter and event mapper already normalized usage-limit failures away from misleading login text.
- The separate session/chat execution path in `sessions.go` still returned raw Claude login-looking failures directly.
- Desktop users could still see `/login` guidance for usage-limited Claude accounts even after BUG-054 was marked done.

### Current Ask

- Apply the same usage-limit normalization to the Claude session execution path used by desktop chat/session prompt execution.

### Key Decisions

- `V-1` Reuse the existing `claudeUsageLimitError` and `claudeUsageLimitMessage` helpers instead of adding a second Claude-specific classifier.
- `V-2` Normalize both the preflight metadata path and the result/stderr error path so the raw `/login` message cannot leak through this execution surface.

### Constraints

- Keep the fix scoped to the Claude session execution path in `sessions.go`.
- Do not change workflow-adapter behavior already covered by BUG-051 and BUG-054.
- GitNexus impact tooling was not available in this thread, so the required symbol impact step was replaced with careful local inspection.

### Open Questions

- Should the Claude error-normalization helpers move into a provider-shared error utility once more Claude execution surfaces are added?

### Source Refs

- User report `2026-06-15`: BUG-054 was not sufficient; desktop still showed the Claude error.
- `apps/local-runner/internal/runner/sessions.go`
- `apps/local-runner/internal/runner/sessions_test.go`
- `apps/local-runner/internal/runner/claude_usage.go`
- `apps/local-runner/internal/runner/claude_event_mapper.go`

## 1. Issue Summary

Desktop Claude prompt execution still surfaced misleading login guidance for usage-limited accounts because the session-based `SendMessage` path never used the Claude quota-normalization helpers that the workflow adapter already used. As a result, BUG-054 corrected one execution surface while the desktop chat/session surface still leaked the old symptom.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: Desktop app connected to the local runner, using the Claude session/chat prompt execution path rather than the workflow-adapter turn path.
- reproduction steps:
  1. Start a Claude-backed desktop session or chat execution.
  2. Use a Claude account whose local auth metadata reports `cachedExtraUsageDisabledReason: "out_of_credits"` or whose result frame carries quota metadata.
  3. Send a prompt through the session execution path.
  4. Observe the returned error text.
- frequency: Reproducible whenever the Claude session execution path hits a usage-limited account and the raw Claude failure looks like login guidance.

## 4. Expected vs Actual

- expected: the session execution path reports Claude usage-limit guidance, such as `Claude usage limit reached. Switch Claude account or wait for quota reset.`
- actual: the session execution path could still return raw login-looking text such as `Not logged in · Please run /login`.

## 5. Impact

- users affected: Desktop users using Claude through session/chat prompt execution.
- workflows affected: Claude session/chat prompt execution, not only the workflow adapter path.
- severity: `medium`, because the runner reports the wrong remediation and makes a quota issue look like an auth issue.

## 6. Root Cause

- hypothesis: BUG-051 and BUG-054 fixed only the workflow-adapter/event-mapper path, while `sessions.go` kept its own direct Claude result handling.
- confirmed cause: `Runner.SendMessageWithCallback` in `sessions.go` handled `claude_stream_json` errors by returning raw stderr/result strings without invoking `claudeUsageLimitError` or `claudeUsageLimitMessage`.
- evidence: local code inspection showed quota normalization was wired in `provider_registry.go`, `claude_usage.go`, and `claude_event_mapper.go`, but not in the `claude_stream_json` branch of `sessions.go`. Focused tests now cover both the metadata-preflight and result-frame cases.

## 7. Fix Strategy

- `F-1` Add Claude usage-limit preflight to the `claude_stream_json` session execution path before starting the print command.
- `F-2` Normalize Claude stderr failures in the session execution path using `claudeUsageLimitMessage`.
- `F-3` Normalize Claude result-frame failures in the session execution path using `claudeUsageLimitMessage`.
- `F-4` Add focused tests for the preflight metadata case and the misleading result-frame case.

## 8. Validation

- `V-1` `go test ./internal/runner -run "Test(SendMessageClaudePreflightUsageLimitDoesNotRunCommand|SendMessageClaudeResultLimitDoesNotReportLogin|MapClaudeLineResultLimitDoesNotReportLogin|ClaudeUsageLimitErrorFromAuthMetadata)$"` - **PASS**

## 9. Regression Guard

- tests: `TestSendMessageClaudePreflightUsageLimitDoesNotRunCommand`, `TestSendMessageClaudeResultLimitDoesNotReportLogin`, `TestMapClaudeLineResultLimitDoesNotReportLogin`, and `TestClaudeUsageLimitErrorFromAuthMetadata`
- alerts: none
- audit checks: `change-audit/CA-068-fix-claude-session-usage-limit-normalization.md`

## 10. Follow-Up Document Updates

- upstream docs that must change: none - this is a code-path parity fix, not a change in provider contract.
- notes left unchanged on purpose: `BUG-054` remains valid for the workflow-adapter message wording fix; this bug records the missing session/chat execution coverage.
