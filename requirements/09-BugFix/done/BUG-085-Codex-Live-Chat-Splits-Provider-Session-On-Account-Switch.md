---
name: BUG-085-Codex-Live-Chat-Splits-Provider-Session-On-Account-Switch
description: A live Codex chat creates separate provider rollout sessions and loses account-B turns when the user switches active Codex accounts inside the same FlowPilot chat.
metadata:
  type: bugfix
---

## Metadata

- Document ID: `BUG-085`
- Title: Codex Live Chat Splits Provider Session On Account Switch
- Phase: `bugfix`
- Status: `done`
- Owner: DatNguyen
- Reviewers: —
- Created: 2026-06-18
- Last Updated: 2026-06-18
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: —
- Related Documents: [BUG-083: Desktop Chat Resume Replays Composed Prompt, Not User Input](../done/BUG-083-Desktop-Chat-Resume-Replays-Composed-Prompt-Not-User-Input.md), [BUG-084: Codex History Chat Open Fails After Switching Active Account](../done/BUG-084-Codex-History-Chat-Open-Fails-After-Switching-Active-Account.md), [Task-067: Desktop Post-Restart Run Resume Via Provider Session ID](../../08-Task/todo/Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md)
- Replaces: —
- Tags: desktop, chat, codex, provider-accounts, live-session, resume, local-runner, severity-high

## AI Quick View

### Summary

- Repro: start one live Codex chat on account A (`aaa`), switch active Codex account to B (`bbb`), switch back to A (`ccc`).
- Follow-up repro from screenshots: one FlowPilot chat shows both account prompts (`hello, im sub9596`, `hi im mealplanner`), but the native Codex app logged into account A only shows the first prompt.
- Actual before fix: account A's Codex desktop history can show separate or stale sessions; account B's turn remains under account B's Codex home and account A does not receive the appended stable rollout content.
- Root cause: live Codex follow-up turns first used app-server `thread/start`; after that was fixed, same-session rollout copies were still treated as conflicts when the destination already existed but was older.
- Fix: live Codex chat follow-ups resume the stable rollout id, chat-only account switches prepare/copy the stable rollout plus recorded sidecar rollout files, and successful turns sync appended stable rollout content back to known same-provider account homes.

### Current Ask

- Fixed in `apps/local-runner/internal/runner/` and guarded by live A -> B -> A plus stale-destination rollout regression tests.

### Key Decisions

- `V-1` Normal chat runs may continue across connected accounts of the same provider.
- `V-2` Workflow runs remain scoped to the account they started with.
- `V-3` Codex must keep the stable durable resume id while recording newer per-turn rollout ids only as transcript/session sidecars.

### Constraints

- Do not overwrite different existing provider session files.
- Do not change BUG-083 transcript filtering rules.
- Do not silently auto-replay interrupted in-flight turns during account switch.

### Open Questions

- None for this fix.

### Source Refs

- User report on 2026-06-18: "start chat with codex - accA - chat aaa; active acc B - chat bbb; active acc A again - chat ccc ... 2 new chat aaa -- ccc created ... lost chat bbb".
- User screenshot report on 2026-06-18: FlowPilot shows two account prompts in one chat, while native Codex logged into account A shows only the first prompt.
- Local evidence on 2026-06-18: `/Users/tiendat/.codex/.../rollout-2026-06-18T20-57-25-019edb05-b5b1-7b53-a550-3a3baa8b6ef9.jsonl` contained only `hello, im sub9596`; `/Users/tiendat/.codexHome1/.../rollout-2026-06-18T20-57-25-019edb05-b5b1-7b53-a550-3a3baa8b6ef9.jsonl` contained both `hello, im sub9596` and `hi im mealplanner`.
- `apps/local-runner/internal/runner/interactive_service.go` -> `startTurn`, `runTurn`, `SetActiveAccount`
- `apps/local-runner/internal/runner/interactive_resume.go` -> `ensureResumeReady`, `prepareCrossAccountResume`
- `apps/local-runner/internal/runner/codex_adapter.go` -> live app-server `thread/start`
- `apps/local-runner/internal/runner/codex_resume_process.go` -> `codex exec resume`
- `apps/local-runner/internal/runner/cross_account_resume_test.go` -> `TestLiveCodexChatResumesAcrossProviderAccountSwitches`, `TestRelocateSessionFileUpdatesOlderCodexDestinationWhenSourceExtends`, `TestSyncCodexStableSessionToKnownAccountsUpdatesOlderHomes`

## 1. Issue Summary

A single FlowPilot normal chat can split into multiple Codex provider sessions or leave stale stable rollout files when the user switches active Codex accounts during the live chat. The visible symptoms are that account A can show separate chats for `aaa` and `ccc`, or the native Codex app for account A can show only the first prompt while FlowPilot shows account A and account B prompts in one chat.

## 2. Parent Links

- impacted coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot, local runner, Codex provider, two connected Codex accounts A and B.
- reproduction steps:
  1. Activate Codex account A.
  2. Start one normal chat and send `aaa`.
  3. Activate Codex account B.
  4. Continue the same FlowPilot chat and send `bbb`.
  5. Activate Codex account A again.
  6. Continue the same FlowPilot chat and send `ccc`.
  7. Open the Codex desktop app with account A.
- alternate screenshot repro:
  1. Activate Codex account A.
  2. Send `hello, im sub9596`.
  3. Activate Codex account B.
  4. Send `hi im mealplanner` in the same FlowPilot chat.
  5. Open native Codex with account A.
- frequency: deterministic from code path.

## 4. Expected vs Actual

- expected: one FlowPilot chat continues the same Codex provider session chain across account A and B, and all rollout files needed for replay/resume are available from each account home that already participates in the session.
- actual before fix: live Codex follow-up turns started fresh provider threads, and the active-account guard rejected switched runs before cross-account preparation. After the first path was fixed, account B could append to the same stable rollout but account A's existing stable rollout copy remained stale because existing non-identical destination files were treated as conflicts.
- actual after fix: live follow-up turns use `codex exec resume <stable rollout id>`, chat account switches run cross-account preparation before the turn, known Codex sidecar rollout files follow the active account, and same-session appended stable rollout content syncs back to known same-provider account homes.

## 5. Impact

- users affected: users with multiple connected Codex accounts who continue one chat while switching the active account.
- workflows affected: live chat continuity, provider-side Codex history, post-restart transcript replay, cross-account history open.
- severity: High — the FlowPilot chat appears continuous, but provider-side state is split and a turn can disappear from the account the user switches back to.

## 6. Root Cause

- hypothesis: cross-account resume was only implemented for post-restart/history open, not for live follow-up turns.
- confirmed cause:
  - `startTurn` returned `409 provider_account_changed` as soon as the active provider account differed from the run's stamped account, so a live chat could not prepare/copy the provider session into the new active account.
  - Live Codex follow-up turns used the app-server adapter, whose `SendTurn` always calls `thread/start`; the `codex exec resume` adapter was only selected for `resumedFromDisk` runs.
  - `runTurn` passed the synthetic `thread-*` id in `TurnRequest.ProviderSessionID`, so even a live-path resume adapter would not have received the real rollout id.
  - `prepareCrossAccountResume` copied only the stable resume file, not the BUG-083 turn-log sidecar rollout ids produced while another account was active.
  - `RelocateSessionFile` accepted an existing destination only when the bytes were identical. In the live screenshot repro, account A had the same stable rollout id but an older file, while account B had the appended file with both turns; the older account A file was rejected instead of being updated.
  - No post-turn sync refreshed older same-provider account homes after `codex exec resume` appended a turn to the current active account's stable rollout file.
- evidence:
  - `codexAdapter.SendTurn` starts a fresh thread each turn.
  - `startTurn` had a pre-adapter `provider_account_changed` conflict.
  - Native Codex account A reads `/Users/tiendat/.codex/sessions/.../rollout-2026-06-18T20-57-25-019edb05-b5b1-7b53-a550-3a3baa8b6ef9.jsonl`, which had only the first prompt.
  - The active account B home `/Users/tiendat/.codexHome1/sessions/.../rollout-2026-06-18T20-57-25-019edb05-b5b1-7b53-a550-3a3baa8b6ef9.jsonl` had the same rollout id plus the second prompt.
  - `TestLiveCodexChatResumesAcrossProviderAccountSwitches` reproduces account A -> B -> A with one live run and verifies the stable resume id plus sidecar copy behavior.
  - `TestRelocateSessionFileUpdatesOlderCodexDestinationWhenSourceExtends` verifies same-session appended rollout content updates an older destination.
  - `TestSyncCodexStableSessionToKnownAccountsUpdatesOlderHomes` verifies a successful Codex turn syncs the stable rollout back to older known account homes.

## 7. Fix Strategy

- `F-1` Allow `runKind == "chat"` to call `ensureResumeReady` when the active same-provider account changed; keep workflow runs scoped with `provider_account_changed`.
- `F-2` Select `codexResumeAdapter` for live Codex follow-up turns once a real rollout id is known, not only for `resumedFromDisk` runs.
- `F-3` Pass the real Codex rollout id into `TurnRequest.ProviderSessionID` so `codex exec resume` receives the stable durable resume handle.
- `F-4` During cross-account preparation, copy recorded Codex turn-log sidecar rollout files from the source account home into the target account home.
- `F-5` Update stale account-switch comments to document the chat-vs-workflow split.
- `F-6` Allow Codex relocation to update an existing destination only when source and destination are the same rollout id, compatible workspace, and one file is a byte prefix of the other.
- `F-7` After a successful Codex turn, best-effort sync the stable rollout file to same-provider accounts that already have that session file, without copying auth data and without creating new session files in unrelated homes.

## 8. Validation

- `V-1` ✅ `go test ./internal/runner -run 'TestRelocateSessionFileUpdatesOlderCodexDestinationWhenSourceExtends|TestSyncCodexStableSessionToKnownAccountsUpdatesOlderHomes|TestLiveCodexChatResumesAcrossProviderAccountSwitches|TestRelocateSessionFileDoesNotOverwriteExistingSessionFile' -count=1`
- `V-2` ✅ `go test ./internal/runner -count=1` passed: 630 tests.
- `V-3` ✅ Regression test verifies account B's `rollout-bbb` is copied back into account A before the `ccc` turn.
- `V-4` ✅ Regression test verifies both B and A follow-up turns call `codex exec resume rollout-aaa`, not the fallback app-server adapter.
- `V-5` ✅ Regression test verifies an older same-session Codex destination file is updated when the source extends it, while divergent existing files still fail.
- `V-6` ✅ Regression test verifies post-turn stable rollout sync updates an older account-home copy that already has the session.

## 9. Regression Guard

- tests: `TestLiveCodexChatResumesAcrossProviderAccountSwitches`, `TestRelocateSessionFileUpdatesOlderCodexDestinationWhenSourceExtends`, `TestSyncCodexStableSessionToKnownAccountsUpdatesOlderHomes`.
- alerts: none.
- audit checks: GitNexus impact analysis was run before editing `startTurn`, `runTurn`, `prepareCrossAccountResume`, `refreshResumeHandleLocked`, `SetActiveAccount`, `resumeSessionID`, `ensureProviderResumeHandle`, and `RelocateSessionFile`; no HIGH/CRITICAL risks were reported.

## 10. Follow-Up Document Updates

- upstream docs that must change: none required for this bug fix; CP-18 already expects provider session continuity and Task-067/BUG-083 cover durable resume handles.
- notes left unchanged on purpose: this fix does not auto-replay turns cancelled during the active-account switch, does not copy auth data between Codex homes, and still rejects divergent existing destination session files.
