# BUG-149: Gemini ConPTY Capture Crashes AGY

## Metadata

- Document ID: `BUG-149`
- Title: `Gemini ConPTY Capture Crashes AGY`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-29`
- Last Updated: `2026-06-29`
- Parent Documents: [Task-167: Gemini Resume Handoff And Live DOD](../../08-Task/inprogress/Task-167-Gemini-Resume-Handoff-And-Live-DOD.md), [CP-40: Gemini Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-40-Gemini-Adapter-Plan.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `None`
- Related Documents: [BUG-148: Gemini Response Printed But Not Shown In UI](BUG-148-Gemini-Response-Printed-But-Not-Shown-In-UI.md), [CA-143: Gemini AGY Conversation DB Recovery](../../../change-audit/CA-143-gemini-agy-conversation-db-recovery.md), [CA-144: Gemini AGY Disable ConPTY Capture](../../../change-audit/CA-144-gemini-agy-disable-conpty-capture.md)
- Replaces: `None`
- Tags: `gemini, ai-providers, agy, windows, crash`

## AI Quick View

### Summary

- After adding Windows ConPTY capture for AGY print output, Gemini turns could crash with `exit status 0xc0000374`.
- The supervisor treated the runner process as exited and shut down the stack.
- The conversation DB fallback from `BUG-148` already recovers empty AGY pipe output without needing ConPTY.
- The fix disables ConPTY capture on Windows and removes the dependency path.

### Current Ask

- Stop Gemini AGY turns from crashing the local runner while preserving UI response recovery.

### Key Decisions

- `V-1` Windows AGY capture uses ordinary buffered process execution only.
- `V-2` Empty buffered output remains acceptable because `BUG-148` recovers the assistant response from AGY's conversation DB.
- `V-3` ConPTY is not used for AGY until a safe implementation is proven separately.

### Constraints

- GitNexus tools were not exposed, so symbol impact was assessed manually.
- The fix must stay Gemini-scoped and avoid changing Codex or Claude capture.
- Full runner package tests have existing unrelated failures documented in `BUG-147`.

### Open Questions

- None for the rollback; future terminal capture experiments should be isolated behind an opt-in live flag.

### Source Refs

- User crash log: `exit status 0xc0000374`, supervisor exited with code 1.
- `apps/local-runner/internal/runner/gemini_agy_capture_windows.go`
- `apps/local-runner/internal/runner/gemini_agy_recovery.go`
- `go test ./internal/runner -run 'TestGeminiAdapter|TestExtractGeminiAgyOutputFromPayload|TestSendMessageGeminiUsesAgyPrint|TestResolvePromptExecutionAdapter|TestSummarize|TestStripTerminalSequences|TestIsAgyCommand' -count=1`

## 1. Issue Summary

Gemini chat crashed the local runner immediately after AGY launch. The log showed `exit status 0xc0000374`, then the supervisor shut down the stack.

## 2. Parent Links

- impacted coding plan: `CP-40`
- impacted tech design: `SD-12`, `SD-06`
- impacted system spec: `SS-11`

## 3. Environment and Reproduction

- environment: Windows local runner, Gemini provider through AGY print mode
- reproduction steps: send a Gemini chat message after enabling the Windows ConPTY capture path
- frequency: observed immediately on a Gemini turn with AGY model `gemini-3.5-flash-low`

## 4. Expected vs Actual

- expected: AGY completes normally; if pipe output is empty, FlowPilot recovers the answer from AGY's conversation DB and keeps the runner alive
- actual: AGY/ConPTY path crashed with `0xc0000374`, and the supervisor shut down the runner stack

## 5. Impact

- users affected: Windows users running Gemini chat through AGY
- workflows affected: Gemini desktop chat, Gemini session sends, Gemini one-shot prompt execution, Gemini summarization if ConPTY path is used
- severity: `critical` for affected users because it terminates the runner process

## 6. Root Cause

- hypothesis: the ConPTY capture path destabilized AGY or the pseudo-console integration on the current Windows AGY build
- confirmed cause: crash appeared only after the ConPTY capture path was introduced; ordinary shell/pipe execution exits cleanly, even though it captures zero bytes
- evidence: user crash log shows failure directly after AGY launch and before a result log; prior live probes showed normal execution with empty stdout/stderr and exit code 0

## 7. Fix Strategy

- `F-1` Replace Windows `captureAgyPrint` with the same buffered process execution used for non-ConPTY paths.
- `F-2` Keep the `BUG-148` conversation DB recovery as the mechanism for empty AGY output on Windows.
- `F-3` Remove ConPTY-specific code references from comments/tests so the capture contract is clear.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'TestGeminiAdapter|TestExtractGeminiAgyOutputFromPayload|TestSendMessageGeminiUsesAgyPrint|TestResolvePromptExecutionAdapter|TestSummarize|TestStripTerminalSequences|TestIsAgyCommand' -count=1`
- `V-2` Live Gemini retry was not run by the agent after the fix; user should restart the local runner and retry the same prompt.

## 9. Regression Guard

- tests: focused Gemini adapter/session/prompt/summarizer tests plus capture helper tests
- alerts: AGY recovery logs still report `recovery=conversation_db` when empty capture is recovered
- audit checks: ConPTY import and dependency path removed from Windows capture code

## 10. Follow-Up Document Updates

- upstream docs that must change: none; this is a crash rollback inside the Gemini adapter implementation
- notes left unchanged on purpose: `BUG-148` remains the source for empty-output recovery behavior
