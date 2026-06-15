# BUG-054: Claude Usage Limit Message Contains Confusing Login Phrase

## Metadata

- Document ID: `BUG-054`
- Title: `Claude usage limit message contains confusing login phrase`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot maintainers`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`, `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-051-Claude-Limit-Reported-As-Login.md`, `change-audit/CA-067-fix-claude-usage-limit-message-trailing-login-phrase.md`
- Replaces: `none`
- Tags: `desktop, local-runner, claude, provider-runtime, quota, ux`

## AI Quick View

### Summary

- After the BUG-051 fix, both quota error message strings ended with `"; login is still present"`.
- That phrase was intended to reassure users their credentials were intact, but it reads as a second active problem alongside the quota exhaustion.
- Users see the full message and interpret "login is still present" as evidence of a login issue.

### Current Ask

- Remove the trailing `"; login is still present"` clause from both quota error message sites so the message is unambiguous.

### Key Decisions

- `V-1` Keep the actionable part: "Switch Claude account or wait for quota reset."
- `V-2` Do not add a login-state sentence; the quota message is already sufficient.

### Constraints

- No change to quota detection logic (`isClaudeUsageLimitReason`, `claudeUsageLimitMessage`, `claudeUsageLimitError`).
- Two existing tests pinned the old strings; they must be updated to match.

### Open Questions

- If Claude CLI exposes a reset-time estimate in result metadata, the message could surface it (see BUG-051 open questions).

### Source Refs

- User report `2026-06-15`: message "…Switch Claude account or wait for quota reset; login is still present" is confusing.
- `apps/local-runner/internal/runner/claude_usage.go`
- `apps/local-runner/internal/runner/claude_event_mapper.go`
- `apps/local-runner/internal/runner/claude_adapter_test.go`

## 1. Issue Summary

Both quota-limit error messages emitted by the local runner ended with `"; login is still present"`. The clause was added in BUG-051 to signal that the user's credentials were intact and the failure was purely quota-driven. In practice it reads as a second concurrent problem — a login issue — not as reassurance. Users repeatedly see the phrase and interpret it as requiring a login action when none is needed.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/07-Claude-Adapter-Plan.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: Desktop app connected to the local Go runner; Claude account with `cachedExtraUsageDisabledReason: "out_of_credits"`.
- reproduction steps:
  1. Select a Claude account that has exhausted available usage.
  2. Run any workflow with Claude as the provider.
  3. Observe the error message in the desktop timeline.
- frequency: Reproducible whenever the Claude account is usage-limited.

## 4. Expected vs Actual

- expected: `Claude usage limit reached: extra usage unavailable (out of credits). Switch Claude account or wait for quota reset`
- actual: `Claude usage limit reached: extra usage unavailable (out of credits). Switch Claude account or wait for quota reset; login is still present`

## 5. Impact

- users affected: Desktop users whose Claude account is usage-limited.
- workflows affected: Any Claude provider run that hits the preflight or result-frame quota check.
- severity: `low`, because the workflow failure was reported correctly; only the message text was misleading.

## 6. Root Cause

- hypothesis: The BUG-051 fix appended `; login is still present` to distinguish quota failure from login failure, but the phrasing is ambiguous in both message sites.
- confirmed cause: `claude_usage.go:claudeUsageLimitError` and `claude_event_mapper.go:claudeUsageLimitMessage` both include the trailing clause in their hardcoded strings.
- evidence: Exact strings visible in both source files; user sees the phrase verbatim in the desktop timeline.

## 7. Fix Strategy

- `F-1` Remove `"; login is still present"` from `claudeUsageLimitError` in `claude_usage.go`.
- `F-2` Remove `"; login is still present."` from `claudeUsageLimitMessage` in `claude_event_mapper.go`.
- `F-3` Update the two test assertions in `claude_adapter_test.go` that pinned the old strings.

## 8. Validation

- `V-1` `go test ./internal/runner -run "Test(MapClaudeLineResultLimitDoesNotReportLogin|ClaudeUsageLimitErrorFromAuthMetadata|ClaudeRegistryGating)$"` — **PASS** (3/3).

## 9. Regression Guard

- tests: `TestMapClaudeLineResultLimitDoesNotReportLogin` and `TestClaudeUsageLimitErrorFromAuthMetadata` now pin the corrected strings.
- alerts: none.
- audit checks: `change-audit/CA-067-fix-claude-usage-limit-message-trailing-login-phrase.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this is a message-text correction only.
- notes left unchanged on purpose: `BUG-051` is the parent fix; this bug is a follow-up polish to its message wording.
