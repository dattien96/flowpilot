# Task-081: Restore Remote Chats Without Active Provider

## Metadata

- Document ID: `Task-081`
- Title: `Restore Remote Chats Without Active Provider`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [Task-069: Cross-PC Sync for Non-Supabase Users](../inprogress/Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md), [Task-073: Cross-PC Non-Supabase Chat Sync Definition of Done Checklist](../inprogress/Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md), [10-IG: Cross-PC Non-Supabase Chat Sync Implementation Guide](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)
- Child Documents: `none`
- Related Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [Task-075: Cross-Account And Cross-PC Chat E2E Test Guide](../inprogress/Task-075-Cross-Account-And-Cross-PC-Chat-E2E-Test-Guide.md)
- Replaces: `none`
- Tags: `desktop, local-runner, chat-sync, restore, provider-account`

## AI Quick View

### Summary

- Remote chat restore no longer requires an active or authenticated account for the chat provider.
- Restored provider files use the active account home when available and the provider's default data home otherwise.
- Restored transcripts can be opened read-only without provider authentication.
- The desktop chat controller disables providers that have no connected account.

### Current Ask

- Completed: restore all remote chats and defer provider availability gating to continuation controls.

### Key Decisions

- `T-1` Provider authentication is not a restore precondition.
- `T-2` Missing provider authentication blocks new turns, not transcript viewing.
- `T-3` Provider session bytes remain in provider-compatible local paths.

### Constraints

- Preserve integrity, path traversal, cwd remap, and session conflict checks.
- Do not weaken turn-time provider authentication checks.
- Do not change provider auth discovery behavior.

### Open Questions

- None.

### Source Refs

- Task-069 `T-3`
- Task-073 `DOD-034` through `DOD-048`
- [10-IG: Cross-PC Non-Supabase Chat Sync Implementation Guide](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)
- [SD-14 cross-PC restore flow](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)

## 1. Goal

Allow PC B to restore and read a synced Claude or Codex chat even when that provider has no active account on PC B.

## 2. Parent Links

- coding plan: [10-IG: Cross-PC Non-Supabase Chat Sync Implementation Guide](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)
- tech design: [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: Task-069 `T-3`, Task-073 `DOD-034` through `DOD-048`

## 3. Trigger

PC B may not have Claude installed or authenticated. The previous active-account precondition prevented restoring Claude history even though the synced session file was available and readable.

## 4. Exact Change

- `T-1` Restore into the active provider account home when resolvable.
- `T-2` Otherwise restore into the provider's default local data home and persist the account pointer as `default`.
- `T-3` Permit history resume to continue in read-only mode for `account_not_signed_in` and `account_unavailable`.
- `T-4` Keep turn execution protected by the existing provider readiness check.
- `T-5` Disable provider selection and sending in the desktop chat controller when no connected account exists.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/session_file_locator.go`
  - `apps/local-runner/internal/runner/chat_session_sync.go`
  - `apps/local-runner/internal/runner/interactive_resume.go`
  - `apps/local-runner/internal/runner/interactive_handlers.go`
  - `apps/local-runner/internal/runner/chat_session_sync_test.go`
  - `apps/local-runner/internal/runner/cross_account_resume_test.go`
  - `apps/desktop-flowpilot/src/components/ChatInput.tsx`
  - `apps/desktop-flowpilot/src/styles.css`
- modules: `local-runner`, `desktop-flowpilot`
- routes: existing restore and resume routes only
- tables: none

## 6. Acceptance Check

- Remote restore succeeds with no registered account for the provider.
- Remote restore succeeds when the target account exists but is not authenticated.
- The restored transcript can be opened without provider authentication.
- Sending remains disabled until a connected account exists.
- Existing integrity, collision, and cwd-remap tests pass.

## 7. Out of Scope

- Installing provider CLIs.
- Creating provider credentials.
- Converting provider session files into a FlowPilot-owned transcript format.
- Changing account activation or quota behavior.

## 8. Completion Notes

- result: implemented provider-independent restore with read-only history opening and desktop provider gating
- follow-ups: manual PC B Claude validation remains useful
- upstream docs updated: Task-073 restore requirements and Task-075 manual cases now define read-only behavior without provider auth
