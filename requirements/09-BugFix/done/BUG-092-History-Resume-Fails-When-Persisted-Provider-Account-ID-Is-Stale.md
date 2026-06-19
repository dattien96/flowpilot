# Metadata

- Document ID: `BUG-092`
- Title: `History Resume Fails When Persisted Provider Account ID Is Stale`
- Phase: `bugfix`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `—`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `—`
- Related Documents: [Task-069: Cross-PC Sync for Non-Supabase Users](../../08-Task/inprogress/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md), [BUG-091: Drive Restore Rejects Same-Session Prefix Extension As Conflict](../done/BUG-091-Drive-Restore-Rejects-Same-Session-Prefix-Extension-As-Conflict.md), [CA-107: Fix Remote Restore Account Resolution After Restart](../../../change-audit/CA-107-fix-remote-restore-account-resolution-after-restart.md)
- Replaces: `—`
- Tags: `google-drive, remote-chat, restore, restart, provider-account, local-runner, severity-high`

## AI Quick View

### Summary

- Synced Claude and Codex chats failed to open after restart when `sessions.ndjson` referenced provider account IDs no longer present in `provider-accounts.json`.
- The exact provider session files still existed under current same-provider account homes.
- Resume stopped at stored-account home resolution and never searched surviving same-provider homes for the exact session ID.
- Resume now recovers the matching account/home, verifies auth, and persists the repaired account ID.

### Current Ask

- Make history resume self-heal stale provider account IDs for both Claude and Codex.

### Key Decisions

- `V-1` Drive restore must use `activeAccountForProvider(manifest.ProviderKey)`.
- `V-2` The restored `ProviderSessionState.ProviderAccountID` must use the same resolved account id as the destination home.
- `V-3` Existing integrity, auth, and session-file conflict checks remain unchanged.
- `V-4` Diagnostic logs must identify reconstruction, account resolution, session-file lookup, HTTP response, and stream replay without logging prompt content or credentials.
- `V-5` Stale-account recovery searches only registered homes of the same provider for the exact persisted provider session ID.

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

After synced chats were restarted, clicking Claude `run-11` or Codex `run-35` in History disabled the row and returned `session_unavailable`, even though their provider session files remained on the machine.

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
- frequency: deterministic when the persisted provider account ID was removed/recreated and no longer resolves

## 4. Expected vs Actual

- expected: history resume finds the exact session under a surviving same-provider account home, repairs `provider_account_id`, and opens the chat
- actual before fix: resume immediately returned `session_unavailable` when the stale stored account ID could not resolve

## 5. Impact

- users affected: users whose Claude or Codex account records were recreated while local provider session files remained
- workflows affected: local History reopen and subsequent continued turns after restart
- severity: high because valid local chat data existed but was inaccessible

## 6. Root Cause

- hypothesis: the remaining failure was still caused by legacy default-account selection
- confirmed cause: both real traces loaded valid provider session IDs but failed at `resolveAccountHome` because the persisted account IDs no longer existed. `ensureResumeReady` returned before searching other registered homes of the same provider.
- evidence:
  - Claude: stored account `9a0ab3...`, active account `eb3e278...`, source home unresolved
  - Codex: stored account `f49ab0...`, active account `c1e581...`, source home unresolved
  - both failures returned `session_unavailable` before provider-file lookup

## 7. Fix Strategy

- `F-1` Resolve `activeAccountID := s.activeAccountForProvider(manifest.ProviderKey)` during Drive restore.
- `F-2` Resolve `targetHome` from that provider-scoped account id.
- `F-3` Persist the same account id in `ProviderSessionState.ProviderAccountID`.
- `F-4` Update the missing-active-account test to remove durable connected accounts instead of only changing the legacy service field.
- `F-5` Add desktop `[FlowPilot][history-open]` logs around resume and stream replay.
- `F-6` Add runner `[chat-history-open]` logs for persisted-run loading, active/stored account resolution, provider session-file lookup, relocation, auth checks, and typed HTTP responses.
- `F-7` When the stored account cannot resolve, scan registered same-provider homes for the exact persisted session ID, preferring the active account.
- `F-8` Rebind and persist the recovered account only after the session file and required local auth are verified.

## 8. Validation

- `V-1` `TestSyncedChatCanResumeAfterServiceRestart` passes.
- `V-2` `TestSyncedChatCanRestoreFromRemoteAfterServiceRestart` fails before the fix and passes after it.
- `V-3` All `TestRestoreChatRunFromDrive*` tests pass, including auth, integrity, conflict, identical-file, and prefix-extension cases.
- `V-4` Full `go test ./internal/runner -count=1` was attempted but remains blocked by unrelated existing environment-sensitive failures in Codex CLI resume shims, compatibility canaries, Windows path parsing, and provider-home skills tests.
- `V-5` `TestResumeRunRecoversStaleCodexAccountIDFromActiveProviderHome` passes.
- `V-6` `TestResumeRunRecoversStaleClaudeAccountIDFromActiveProviderHome` passes.

## 9. Regression Guard

- tests: `bug092_test.go` covers restart restore; `cross_account_resume_test.go` covers stale Claude and Codex account-ID recovery
- alerts: preserve typed `account_unavailable` and `account_not_signed_in` responses for genuine provider-account failures
- audit checks: verify destination home and persisted `ProviderAccountID` are derived from the same provider-scoped account

## 10. Follow-Up Document Updates

- upstream docs that must change: SD-14 should eventually state that provider account IDs are mutable pointers and exact provider session files are the durable recovery evidence
- notes left unchanged on purpose: recovery never guesses by newest file and never searches another provider
