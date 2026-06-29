# BUG-148: Gemini Response Printed But Not Shown In UI

## Metadata

- Document ID: `BUG-148`
- Title: `Gemini Response Printed But Not Shown In UI`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-29`
- Last Updated: `2026-06-29`
- Parent Documents: [Task-167: Gemini Resume Handoff And Live DOD](../../08-Task/inprogress/Task-167-Gemini-Resume-Handoff-And-Live-DOD.md), [CP-40: Gemini Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-40-Gemini-Adapter-Plan.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `None`
- Related Documents: [BUG-147: Gemini One-Shot Prompt Returns Empty Response](BUG-147-Gemini-One-Shot-Prompt-Returns-Empty-Response.md), [CA-140: Gemini AGY Runtime Fallback](../../../change-audit/CA-140-gemini-agy-runtime-fallback.md), [CA-142: Gemini One-Shot Prompt No New Project](../../../change-audit/CA-142-gemini-one-shot-prompt-no-new-project.md), [CA-143: Gemini AGY Conversation DB Recovery](../../../change-audit/CA-143-gemini-agy-conversation-db-recovery.md)
- Replaces: `None`
- Tags: `gemini, ai-providers, desktop-chat, agy, windows`

## AI Quick View

### Summary

- AGY printed a valid Gemini answer in the runner console, but FlowPilot logged `stdout_bytes=0`.
- Desktop chat only renders `message_completed.text` / `turn_completed.finalMessage`, so the UI stayed empty.
- Windows AGY print mode can bypass normal stdout/stderr capture even with shell redirection or ConPTY fallback.
- The fix keeps normal capture first, then recovers the latest assistant response from AGY's workspace conversation DB when capture is empty.

### Current Ask

- Make Gemini responses visible in desktop chat when AGY writes the answer outside normal stdout/stderr.

### Key Decisions

- `V-1` Normal stdout remains authoritative when present.
- `V-2` Empty Gemini capture falls back to AGY's `last_conversations.json` workspace mapping and latest assistant `steps.step_payload` entry.
- `V-3` Recovery is Gemini-only and does not change Codex or Claude execution.

### Constraints

- GitNexus tools were not exposed, so symbol impact was assessed manually.
- Recovery currently uses local `sqlite3` when available because AGY persists conversation rows in SQLite.
- Do not parse AGY transcript JSONL because the observed transcript file was zero bytes.

### Open Questions

- Whether future AGY builds will expose a stable JSON or output-file print mode that can replace SQLite fallback.

### Source Refs

- User log showing `stdout_bytes=0` while AGY printed "Hello! I am ready..."
- `apps/local-runner/internal/runner/gemini_adapter.go`
- `apps/local-runner/internal/runner/gemini_agy_recovery.go`
- `apps/local-runner/internal/runner/gemini_agy_capture_windows.go`
- `go test ./internal/runner -run 'TestGeminiAdapter|TestExtractGeminiAgyOutputFromPayload|TestSendMessageGeminiUsesAgyPrint|TestResolvePromptExecutionAdapter|TestSummarize' -count=1`

## 1. Issue Summary

Gemini produced a response, but desktop chat did not show it. The runner log showed AGY's response text followed by `stdout_bytes=0`, proving the process completed successfully while FlowPilot captured an empty final message.

## 2. Parent Links

- impacted coding plan: `CP-40`
- impacted tech design: `SD-12`, `SD-06`
- impacted system spec: `SS-11`

## 3. Environment and Reproduction

- environment: Windows local runner, Gemini provider through AGY print mode, desktop chat
- reproduction steps: send a Gemini chat message; observe AGY answer in runner logs but no assistant message in UI
- frequency: observed on Windows when AGY writes print-mode output through console paths that are not captured by stdout/stderr pipes

## 4. Expected vs Actual

- expected: the answer shown by AGY becomes `message_completed.text` and `turn_completed.finalMessage`
- actual: FlowPilot emitted an empty final message because captured stdout and stderr were both empty

## 5. Impact

- users affected: Gemini desktop chat users on affected Windows AGY builds
- workflows affected: desktop Gemini chat turns, legacy Gemini session sends, one-shot Gemini prompt execution, Gemini summarization
- severity: `high` because the model appears to answer but the UI has no visible response

## 6. Root Cause

- hypothesis: AGY print mode writes the answer through a Windows console output path that bypasses Go stdout/stderr capture
- confirmed cause: live probes showed PowerShell redirection captured `OUT_BYTES=0` and `ERR_BYTES=0`; the same answer was persisted in AGY's conversation SQLite payload for the workspace
- evidence: `last_conversations.json` mapped `D:\working\gate-sandbox` to a conversation id, and the latest `steps.step_payload` row contained the assistant Markdown response as embedded protobuf text

## 7. Fix Strategy

- `F-1` Add `recoverGeminiAgyLatestMessage` to map workspace -> AGY conversation DB and recover the latest assistant payload when capture is empty.
- `F-2` Add protobuf-style length-delimited string extraction for AGY `step_payload` blobs.
- `F-3` Apply recovery after empty Gemini capture in adapter chat turns, session sends, `Runner.ExecutePrompt`, and Gemini summarization.
- `F-4` Keep Codex and Claude paths unchanged.

## 8. Validation

- `V-1` `go test ./internal/runner -run 'TestGeminiAdapter|TestExtractGeminiAgyOutputFromPayload|TestSendMessageGeminiUsesAgyPrint|TestResolvePromptExecutionAdapter|TestSummarize' -count=1`
- `V-2` Manual live probes confirmed normal shell redirection captured zero bytes from AGY print mode on this Windows machine.
- `V-3` Full `go test ./internal/runner -count=1` was not rerun after this fix; earlier full package run still had unrelated existing failures documented in `BUG-147`.

## 9. Regression Guard

- tests: `TestGeminiAdapterRecoversEmptyAgyStdoutFromConversationDB`, `TestExtractGeminiAgyOutputFromPayload`
- alerts: `[gemini-agy]` logs now include `recovery=conversation_db` and recovered byte count when fallback is used
- audit checks: fix is scoped to Gemini AGY capture/recovery paths

## 10. Follow-Up Document Updates

- upstream docs that must change: none; this restores expected Gemini UI response rendering
- notes left unchanged on purpose: AGY capability parity remains under `Task-167`; future work should replace SQLite fallback if AGY adds a stable output mode
