# Metadata

- Document ID: `BUG-092`
- Title: `Remote Chat Restore Uses Legacy Default Account After Restart`
- Phase: `bugfix`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `—`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `—`
- Related Documents: [Task-069: Cross-PC Sync for Non-Supabase Users](../../08-Task/inprogress/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md), [BUG-091: Drive Restore Rejects Same-Session Prefix Extension As Conflict](./BUG-091-Drive-Restore-Rejects-Same-Session-Prefix-Extension-As-Conflict.md), [CA-107: Fix Remote Restore Account Resolution After Restart](../../../change-audit/CA-107-fix-remote-restore-account-resolution-after-restart.md)
- Replaces: `—`
- Tags: `google-drive, remote-chat, restore, restart, provider-account, local-runner, severity-high`

## AI Quick View

### Summary

- A locally synced chat can resume from History after a runner restart, but restoring the same chat from Remote Chats can fail with `account_unavailable`.
- A restarted `InteractiveService` resets its legacy `activeAccountID` to `"default"`.
- `restoreChatRunFromDrive` used that legacy value instead of resolving the durable active account for the manifest provider.
- The Remote Chats button temporarily disables during the request, then re-enables without opening the chat when restore/open fails.

### Current Ask

- Make Drive restore resolve and persist the active account per provider, matching normal history resume behavior.

### Key Decisions

- `V-1` Drive restore must use `activeAccountForProvider(manifest.ProviderKey)`.
- `V-2` The restored `ProviderSessionState.ProviderAccountID` must use the same resolved account id as the destination home.
- `V-3` Existing integrity, auth, and session-file conflict checks remain unchanged.

### Constraints

- Do not overwrite the in-progress BUG-091 prefix-extension work.
- Keep restore scoped to the provider in the manifest; a different provider's active account must not affect destination selection.

### Open Questions

- None.

### Source Refs

- User report on 2026-06-19.
- `apps/local-runner/internal/runner/chat_session_sync.go`
- `apps/local-runner/internal/runner/interactive_resume.go`
- `apps/local-runner/internal/runner/bug092_test.go`

## 1. Issue Summary

After a chat is synced to Drive and the runner is restarted, clicking the chat in Remote Chats can disable the row temporarily and then leave the chat unopened. Directly opening the local History entry remains functional.

## 2. Parent Links

- impacted coding plan: [Task-069](../../08-Task/inprogress/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md)
- impacted tech design: [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- impacted system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot, local runner, provider account stored under a non-default managed home, Google Drive chat sync enabled
- reproduction steps:
  1. Create a chat and sync it to Drive.
  2. Restart the runner/server.
  3. Open Remote Chats.
  4. Click the synced chat to restore/open it.
- frequency: deterministic when the provider's durable active account id is not `"default"` and the legacy default home cannot resolve

## 4. Expected vs Actual

- expected: restore writes or validates the provider file under the active account for the chat's provider, persists that account id, and opens the chat
- actual: restore resolves account `"default"` after restart and returns `account_unavailable: active account home not found`

## 5. Impact

- users affected: users with managed Codex or Claude account homes using Drive-backed Remote Chats
- workflows affected: remote restore after runner restart
- severity: high because a valid synced chat appears available but cannot be opened through the restore workflow

## 6. Root Cause

- hypothesis: synchronization damaged the persisted chat metadata
- confirmed cause: synchronization and direct post-restart history resume work; the failure is isolated to `restoreChatRunFromDrive`, which called `s.ActiveAccount()` for destination lookup and persisted ownership. A new service initializes this legacy field as `"default"`, while `Runner.ResolveProviderAccount` contains the durable provider-scoped active account.
- evidence:
  - `TestSyncedChatCanResumeAfterServiceRestart` passes before the fix
  - `TestSyncedChatCanRestoreFromRemoteAfterServiceRestart` fails before the fix with `account_unavailable`
  - normal resume already uses `activeAccountForProvider`

## 7. Fix Strategy

- `F-1` Resolve `activeAccountID := s.activeAccountForProvider(manifest.ProviderKey)` during Drive restore.
- `F-2` Resolve `targetHome` from that provider-scoped account id.
- `F-3` Persist the same account id in `ProviderSessionState.ProviderAccountID`.
- `F-4` Update the missing-active-account test to remove durable connected accounts instead of only changing the legacy service field.

## 8. Validation

- `V-1` `TestSyncedChatCanResumeAfterServiceRestart` passes.
- `V-2` `TestSyncedChatCanRestoreFromRemoteAfterServiceRestart` fails before the fix and passes after it.
- `V-3` All `TestRestoreChatRunFromDrive*` tests pass, including auth, integrity, conflict, identical-file, and prefix-extension cases.
- `V-4` Full `go test ./internal/runner -count=1` was attempted but remains blocked by unrelated existing environment-sensitive failures in Codex CLI resume shims, compatibility canaries, Windows path parsing, and provider-home skills tests.

## 9. Regression Guard

- tests: `apps/local-runner/internal/runner/bug092_test.go` covers both direct history resume and Remote Chats restore across a service restart
- alerts: preserve typed `account_unavailable` and `account_not_signed_in` responses for genuine provider-account failures
- audit checks: verify destination home and persisted `ProviderAccountID` are derived from the same provider-scoped account

## 10. Follow-Up Document Updates

- upstream docs that must change: none; SD-14 already requires restore into the active provider account home
- notes left unchanged on purpose: desktop restore loading-state behavior is unchanged because the backend restore/open failure is corrected at its source
