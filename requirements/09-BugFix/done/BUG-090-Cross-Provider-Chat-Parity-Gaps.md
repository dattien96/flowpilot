# BUG-090: Cross-Provider Chat Parity Gaps

## Metadata

- Document ID: `BUG-090`
- Title: `Cross-Provider Chat Parity Gaps`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [Task-018: Auto Switch Account On Provider Usage Limit](../../08-Task/done/Task-018-Auto-Switch-Account.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [SD-15: Claude Cross-Account And Cross-PC Chat Resume And Home Sync](../../06-System-Tech-Design/SD-15-Claude-Cross-Acc-Pc-Sync-Design.md)
- Child Documents: `none`
- Related Documents: [BUG-089: Remote Restore UX And Provider Model Auto Select](./BUG-089-Remote-Restore-UX-And-Provider-Model-Auto-Select.md), [CA-104: Fix Cross-Provider Chat Parity Gaps](../../../change-audit/CA-104-fix-cross-provider-chat-parity-gaps.md)
- Replaces: `none`
- Tags: `desktop, local-runner, claude, codex, remote-chat, model-selection, account-switch`

## AI Quick View

### Summary

- Claude usage-limit account switching was implemented but `Task-018` still described it as excluded.
- Remote Chats discarded records whose source machine used a different local project id, even though the selected Drive root already scoped the project.
- Opening a history run changed the provider but retained the previous provider's selected model.

### Current Ask

- Restore provider parity for account switching, Drive-backed remote discovery, and history model selection.

### Key Decisions

- `V-1` Claude fallback account selection remains deterministic by slot when numeric quota telemetry is unavailable.
- `V-2` The configured chat-sync Drive root is the remote project boundary; source-machine project ids are metadata, not a list filter.
- `V-3` History open selects the resumed provider's configured default model.

### Constraints

- Do not invent Claude quota percentages.
- Do not list records outside the selected Drive root.
- Do not alter the persisted model of the historical provider session; only synchronize the desktop control for the next turn.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/state/store.test.ts`
- `apps/local-runner/internal/runner/chat_session_sync.go`
- `apps/local-runner/internal/runner/chat_session_sync_test.go`
- [Task-018](../../08-Task/done/Task-018-Auto-Switch-Account.md)

## 1. Issue Summary

Three related provider-parity defects made Claude behavior appear incomplete: stale task documentation said Claude switching was out of scope, Remote Chats could hide cross-PC Claude records, and opening an old chat could leave a model selected from the previously active provider.

## 2. Parent Links

- impacted coding plan: `none identified`
- impacted tech design: [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [SD-15](../../06-System-Tech-Design/SD-15-Claude-Cross-Acc-Pc-Sync-Design.md)
- impacted system spec: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)

## 3. Environment and Reproduction

- environment: desktop app with Codex and Claude accounts, Drive chat sync enabled
- reproduction steps:
  1. Exhaust an active Claude account while another connected Claude account exists.
  2. Open Remote Chats where the Drive index contains records created under another machine-local project id.
  3. Select Codex and then open a Claude run from history.
- frequency: deterministic for the affected state combinations

## 4. Expected vs Actual

- expected: Claude offers a confirmed fallback switch and retries the original turn; all records in the selected chat-sync Drive root are listed; history open selects a model valid for the resumed provider.
- actual: documentation denied Claude support; cross-machine records with different project ids were filtered out; the prior provider's model remained selected.

## 5. Impact

- users affected: users with multiple Claude accounts or cross-PC Drive chat sync
- workflows affected: usage-limit recovery, Remote Chats restore, old-chat continuation
- severity: `high` because hidden remote sessions and invalid provider/model combinations block continuation

## 6. Root Cause

- hypothesis: Codex-first implementation assumptions leaked into shared provider workflows.
- confirmed cause:
  - `Task-018` was not updated after Claude null-quota fallback logic and tests were implemented.
  - `listRemoteChatSessions` compared remote `record.ProjectID` to the current machine's project id even though the Drive root was already project-scoped.
  - `openHistoryRun` set `selectedProvider` without recalculating `selectedModel`.
- evidence: focused source inspection and regression tests for each path.

## 7. Fix Strategy

- `F-1` Correct `Task-018` to document Claude's deterministic connected-account fallback and mark the implemented task done.
- `F-2` Remove the machine-local project-id filter from remote index listing while retaining the selected Drive root boundary.
- `F-3` Set both provider and provider-default model when opening a history run.
- `F-4` Add regression tests for Claude original-turn retry, mixed-provider remote records with different project ids, and history provider/model synchronization.

## 8. Validation

- `V-1` Desktop TypeScript typecheck passed.
- `V-2` Bundled desktop store tests passed: `21/21`.
- `V-3` Targeted local-runner Remote Chats tests passed.
- `V-4` Full local-runner package was attempted; unrelated environment-sensitive Codex CLI, compatibility canary, Windows path, and provider-home skill tests remain failing.

## 9. Regression Guard

- tests:
  - `openHistoryRun selects the resumed provider default model`
  - `Claude account switch retries the original failed turn`
  - `TestListRemoteChatSessionsIncludesRecordsFromSameDriveRootWithDifferentProjectIDs`
- alerts: `none`
- audit checks: verify future Drive index filtering uses the configured root or a portable project identity, not a machine-local id.

## 10. Follow-Up Document Updates

- upstream docs that must change: `Task-018` corrected in this fix.
- notes left unchanged on purpose: Claude numeric quota ranking remains unsupported until reliable telemetry exists.
