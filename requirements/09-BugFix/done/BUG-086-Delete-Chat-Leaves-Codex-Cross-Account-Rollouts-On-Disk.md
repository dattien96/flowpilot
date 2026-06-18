---
name: BUG-086-Delete-Chat-Leaves-Codex-Cross-Account-Rollouts-On-Disk
description: Deleting a Codex chat removed the FlowPilot history row but left later per-turn rollout files in copied account homes because delete only removed the stable provider_session_id file and ignored Codex turn-log rollout ids.
metadata:
  type: bugfix
---

## Metadata

- Document ID: `BUG-086`
- Title: Delete Chat Leaves Codex Cross-Account Rollouts On Disk
- Phase: `bugfix`
- Status: `done`
- Owner: DatNguyen
- Reviewers: —
- Created: 2026-06-18
- Last Updated: 2026-06-18
- Parent Documents: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: —
- Related Documents: [BUG-085: Codex Live Chat Splits Provider Session On Account Switch](./BUG-085-Codex-Live-Chat-Splits-Provider-Session-On-Account-Switch.md), [CA-100: Delete Chat History](../../../change-audit/CA-100-delete-chat-history.md), [CA-101: Fix Chat Resume Composed Prompt And Codex Multi-Rollout](../../../change-audit/CA-101-fix-chat-resume-composed-prompt-and-codex-multi-rollout.md)
- Replaces: —
- Tags: desktop, delete, codex, provider-accounts, cleanup, local-runner, severity-medium

## AI Quick View

### Summary

- Symptom: deleting a Codex chat removed the FlowPilot history entry but rollout files still remained under copied Codex homes such as `/Users/tiendat/.codexHome2/sessions`.
- Root cause: `deleteChatSession` deleted only `session.ProviderSessionID`; Codex multi-turn chats can also have later rollout ids stored in the per-run turn log.
- Fix: collect the stable session id plus every `kind:"codex_session"` id from the turn log, then delete all matching rollout files across all registered Codex account homes before removing the turn log itself.

### Current Ask

- Ensure chat deletion cleans up the full Codex rollout chain, not just the stable resume file.

### Key Decisions

- `V-1` Keep deletion best-effort for filesystem cleanup, but expand the candidate set to all known Codex rollout ids for the run.
- `V-2` Read the turn log before deleting it so the extra rollout ids remain available to the cleanup path.
- `V-3` Do not broaden cleanup across providers; only the run's own provider homes are scanned.

### Constraints

- Do not delete rollout files from unrelated providers.
- Do not require Google Drive cleanup for this local delete bug.
- Preserve existing no-op behavior when the turn log does not exist.

### Open Questions

- None.

### Source Refs

- User report 2026-06-18: deleted chat but still saw rollout data under `/Users/tiendat/.codexHome2/sessions`.
- `apps/local-runner/internal/runner/interactive_resume.go`
- `apps/local-runner/internal/runner/turn_log.go`
- `apps/local-runner/internal/runner/local_file_session_store.go`
- `apps/local-runner/internal/runner/cross_account_resume_test.go`
- GitNexus impact: `deleteChatSession` LOW risk, 1 direct caller, 0 affected processes.

## 1. Issue Summary

Deleting a desktop Codex chat removed the FlowPilot history row and turn-log file, but provider rollout files created by cross-account continuation could remain on disk in one or more Codex homes. The leftover files were most visible in copied homes such as `/Users/tiendat/.codexHome2/sessions`, making deletion appear incomplete.

## 2. Parent Links

- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted tech design: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot, local runner, Codex provider, one deleted chat that previously continued across multiple Codex accounts
- reproduction steps:
  1. Start a Codex chat.
  2. Continue it across another Codex account so extra rollout files are recorded in the run turn log.
  3. Delete the chat from the Navigator sidebar.
  4. Inspect the copied Codex home `sessions/` directories.
- frequency: deterministic from code when the run has later `codex_session` entries beyond the stable `provider_session_id`

## 4. Expected vs Actual

- expected: deleting the chat removes the FlowPilot history row, the turn log, and every local Codex rollout file that belongs to that run across registered Codex account homes
- actual before fix: only the stable `provider_session_id` rollout file was targeted; later per-turn rollout ids remained on disk

## 5. Impact

- users affected: users who delete Codex chats that were resumed or continued across account homes
- workflows affected: local disk cleanup after chat deletion
- severity: Medium — runtime behavior is not broken, but local deletion is incomplete and leaves orphaned provider state

## 6. Root Cause

- hypothesis: the delete path ignores Codex sidecar rollout ids
- confirmed cause:
  1. `deleteChatSession` resolved the stored `ProviderSessionState`
  2. it scanned all registered accounts of the run's provider
  3. it called `LocateSessionFile(...)` only for `session.ProviderSessionID`
  4. Codex multi-turn chats can also have later rollout ids stored as `kind:"codex_session"` in `<runId>-turns.ndjson`
  5. the turn log was deleted after provider cleanup, so those extra ids were never consulted

## 7. Fix Strategy

- `F-1` Add a helper to collect deletion candidates: the stable `ProviderSessionID` plus all Codex `turnLogKindCodexSession` ids from the run turn log
- `F-2` Iterate every candidate id across all registered account homes for the matching provider and remove every located rollout file
- `F-3` Keep provider-file deletion best-effort and leave store cleanup behavior unchanged

## 8. Validation

- `V-1` ✅ `go test ./internal/runner -run 'TestDeleteChatSessionRemovesCodexStableAndTurnLogRolloutsAcrossAccounts' -count=1 -v`
- `V-2` ✅ verified the session row is removed from the local store after delete
- `V-3` ✅ verified the turn-log sidecar is removed after delete
- `V-4` ✅ verified both stable and later Codex rollout files are removed from both account homes in the regression test

## 9. Regression Guard

- tests: `TestDeleteChatSessionRemovesCodexStableAndTurnLogRolloutsAcrossAccounts`
- audit checks: GitNexus impact run on `deleteChatSession` reported LOW risk; no HIGH/CRITICAL risk was ignored

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: Google Drive synced chat-session files are still outside this local delete path and remain a separate cleanup task
