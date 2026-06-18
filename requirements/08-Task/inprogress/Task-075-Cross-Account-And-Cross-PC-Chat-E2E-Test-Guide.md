# Task-075: Cross-Account And Cross-PC Chat E2E Test Guide

## Metadata

- Document ID: `Task-075`
- Title: `Cross-Account And Cross-PC Chat E2E Test Guide`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-18`
- Last Updated: `2026-06-18`
- Parent Documents: [Task-071: Cross-Account Chat Resume Definition of Done Checklist](./Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md), [Task-073: Cross-PC Non-Supabase Chat Sync Definition of Done Checklist](./Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md), [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md), [10-IG: Cross-PC Non-Supabase Chat Sync Implementation Guide](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)
- Child Documents: `none`
- Related Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [Task-072: Cross-Account Chat Resume Test Signatures](./Task-072-Cross-Account-Chat-Resume-Test-Signatures.md), [Task-074: Cross-PC Non-Supabase Chat Sync Test Signatures](./Task-074-Cross-PC-Non-Supabase-Chat-Sync-Test-Signatures.md), [Task-059: Desktop Check Version Tested Baseline Config](../done/Task-059-Desktop-Check-Version-Tested-Baseline-Config.md), [Task-067: Desktop Post-Restart Run Resume Via Provider Session ID](./Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md), [Task-068: Desktop History Unified View; Account ID As Local-File Pointer](./Task-068-Desktop-History-Unified-View-Account-As-Local-File-Pointer.md), [Task-069: Cross-PC Sync for Non-Supabase Users](./Task-069-Cross-PC-Sync-Non-Supabase-Sessions-Ndjson.md), [Sync_History_note](./Sync_History_note.md)
- Replaces: `none`
- Tags: `desktop, e2e, manual-test, computer-use, codex, claude, history, resume, sync, cross-account, cross-pc`

## AI Quick View

### Summary

- This file is the end-to-end test script for the new history resume and non-Supabase cross-PC sync flow.
- It is written for both human manual QA and Codex Computer Use style automation.
- It turns the remaining manual checks from Task-071 and Task-073 into concrete click-by-click steps.
- It covers both supported providers in scope for this feature: Codex and Claude.

### Current Ask

- Use this guide to validate Phase 1 and Phase 2 behavior from UI startup through resume, sync, restore, greyout, and compatibility checks.

### Key Decisions

- `T-1` Always start tests in desktop `Chat` mode, not `Workflow`.
- `T-2` Use a selected project before any provider chat action.
- `T-3` Use provider/model pairs fixed for the test run:
  - Codex: `Provider = Codex`, `Model = gpt-4.1-mini`
  - Claude: `Provider = Claude`, `Model = Haiku`
- `T-4` Check Version must be run as part of E2E readiness because Task-059 now includes portability canaries for copy/resume assumptions.
- `T-5` Greyout behavior is a pass condition, not a failure, for unsupported or intentionally broken resume cases.

### Constraints

- Do not use real macOS credentials for automated test harnesses; only manual/provider validation should touch real provider accounts.
- Do not change provider session files by hand except in the explicit missing-file negative tests.
- Do not use `Workflow` mode for these cases.
- Do not treat Claude cross-PC portability as guaranteed; record pass/fail/provider-untested exactly as observed.

### Open Questions

- `Q-1` Claude cross-account and cross-PC portability still depend on live provider behavior and may remain `provider-untested`.
- `Q-2` Some desktop prompts such as system restart confirmation are browser-native dialogs and may need a Computer Use confirm step.

### Source Refs

- [Task-071](./Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md)
- [Task-072](./Task-072-Cross-Account-Chat-Resume-Test-Signatures.md)
- [Task-073](./Task-073-Cross-PC-Non-Supabase-Chat-Sync-DOD-Checklist.md)
- [Task-074](./Task-074-Cross-PC-Non-Supabase-Chat-Sync-Test-Signatures.md)
- [Task-059](../done/Task-059-Desktop-Check-Version-Tested-Baseline-Config.md)
- [09-IG](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md)
- [10-IG](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)

## 1. Goal

Provide one complete E2E test script for validating:

- same-account post-restart chat resume
- same-PC cross-account resume
- Drive-backed cross-PC sync and restore
- disabled history behavior for missing or unusable provider data
- compatibility baselines that guard provider-file portability assumptions

## 2. Parent Links

- coding plan: [09-IG: Cross-Account Chat Resume Implementation Guide](../../10-Refactor/New-System/09-Cross-Account-Chat-Resume-Implementation-Guide.md)
- coding plan: [10-IG: Cross-PC Non-Supabase Chat Sync Implementation Guide](../../10-Refactor/New-System/10-Cross-PC-Non-Supabase-Chat-Sync-Implementation-Guide.md)
- specific upstream ids: Task-059, Task-067, Task-068, Task-069, Task-071, Task-072, Task-073, Task-074

## 3. Trigger

Phase 1 and Phase 2 automated coverage is in place, but the remaining acceptance scope is still mostly manual: real provider resume, real account switching, real Drive restore, and UI-level disabled-history behavior. This guide exists so those tests can be run consistently by a human tester or an automation operator without missing setup steps.

## 4. Exact Change

### Environment Prerequisites

- [x] `E2E-001` Prepare one FlowPilot desktop environment with at least one visible project in the `Projects` rail.
- [x] `E2E-002` Prepare one signed-in Codex account A and one signed-in Codex account B on the same machine if cross-account Codex testing is required.
- [ ] `E2E-003` Prepare one signed-in Claude account A and one signed-in Claude account B on the same machine if Claude cross-account testing is required.
- [x] `E2E-004` Prepare Google Drive connection for the selected project if cross-PC sync tests are required.
- [ ] `E2E-005` For cross-PC validation, prepare PC1 and PC2 or two isolated environments that do not share the same provider-home directories.
- [x] `E2E-006` Record the exact provider/model/acc pairs used:
  - Codex: `gpt-5.4-mini` Account Active: `claudesub9596@gmail.com` and `photohl96@gmail.com`
  - Claude: `Haiku`

### Common Start Sequence

- [x] `E2E-010` Launch the desktop app and wait until the main workspace is visible.
- [x] `E2E-011` In the left rail, confirm the `Projects` section is visible.
- [x] `E2E-012` Click the target project so it becomes the active project.
- [x] `E2E-013` In the right rail `Mode` section, select `Chat` from the `Chat mode` tabs.
- [x] `E2E-014` In the provider controls, click the `Codex` or `Claude` provider chip under `Provider`.
- [x] `E2E-015` In the `Model` dropdown, choose the required model for that provider:
  - Codex run: `gpt-5.4-mini`
  - Claude run: `Haiku`
- [x] `E2E-016` Verify the main composer is enabled only after both a project and provider are selected.
- [x] `E2E-017` Use a short deterministic seed prompt for every new run, for example `Remember token E2E-<case-id> and reply with that token only.`

### Case Group A - Baseline Chat And History Creation

- [x] `E2E-020` Start a new Codex chat.
  - steps:
    - select project
    - select `Chat`
    - select `Codex`
    - select model `gpt-5.4-mini`
    - send the seed prompt
  - expected:
    - a timeline appears
    - one history item appears in `History`
    - the history item title matches the sent prompt or provider reply summary

- [ ] `E2E-021` Start a new Claude chat.
  - steps:
    - select project
    - select `Chat`
    - select `Claude`
    - select model `Haiku`
    - send the seed prompt
  - expected:
    - a timeline appears
    - one history item appears in `History`
    - the history item title matches the sent prompt or provider reply summary

### Case Group B - Same-Account Post-Restart Resume

- [x] `E2E-030` Codex same-account post-restart resume.
  - steps:
    - complete `E2E-020`
    - note the history item in `History`
    - click `↻ Restart system`
    - accept the confirmation dialog
    - wait for the app to reconnect
    - reselect the same project if needed
    - in `History`, click the original Codex run
    - send `What token did I ask you to remember?`
  - expected:
    - the old run opens, not a new unrelated run
    - the timeline is rebuilt for that run
    - Codex answers using the earlier context
    - no `run_not_found` error appears

- [ ] `E2E-031` Claude same-account post-restart resume.
  - steps:
    - complete `E2E-021`
    - click `↻ Restart system`
    - accept the confirmation dialog
    - wait for reconnect
    - reselect project if needed
    - in `History`, click the original Claude run
    - send `What token did I ask you to remember?`
  - expected:
    - the old run opens
    - the earlier context is preserved
    - no crash or empty timeline reset occurs

### Case Group C - Same-PC Cross-Account Resume

- [x] `E2E-040` Codex cross-account resume on one PC.
  - steps:
    - start a Codex run while account A is active
    - complete at least one turn
    - switch the active Codex account to account B in `ProviderAccountsPanel`
    - restart the system or reload the run list
    - click the original history item from account A
    - send `Reply with the remembered token and say active-account-switched.`
  - expected:
    - the same history item stays visible
    - the same logical `run_id` is reopened
    - the follow-up turn succeeds under account B if portability is accepted
    - if provider rejects portability, the item becomes disabled with a clear reason instead of crashing

- [ ] `E2E-041` Claude cross-account resume on one PC.
  - steps:
    - start a Claude run while account A is active
    - complete at least one turn
    - switch the active Claude account to account B
    - click the original history item
    - send the follow-up prompt
  - expected:
    - record one of:
      - `pass`: follow-up works under account B
      - `greyout-safe`: item becomes disabled with reason
      - `provider-untested`: real Claude account test could not be executed

### Case Group D - Negative Resume And Greyout

- [ ] `E2E-050` Missing provider session file produces disabled history item.
  - steps:
    - create one resumable chat run
    - stop the system
    - remove or rename the underlying provider session file for that run
    - restart the system
    - select the same project
    - inspect the history row for that run
  - expected:
    - the item remains in `History`
    - the item is disabled/greyed-out
    - the tooltip or title shows a reason such as `session data not found on this machine`
    - clicking does not clear the current timeline and does not crash

- [ ] `E2E-051` Missing active-account auth produces disabled history item.
  - steps:
    - create one cross-account-resumable run
    - sign out or invalidate the target active account
    - restart if needed
    - click the history item that requires the inactive account
  - expected:
    - the item remains visible
    - the item is disabled with a signed-out/account-unavailable reason
    - no new run is silently created

### Case Group E - Check Version And Portability Canaries

- [x] `E2E-060` Fast Check Version passes or gives only understood warnings.
  - steps:
    - open `Settings`
    - open `Check Version`
    - leave `deep run` unchecked
    - click `Run Checks`
  - expected:
    - results include version checks for Claude Code CLI and Codex
    - results include session-portability checks for Codex resume surface, Codex rollout metadata, and Claude session-store presence
    - if only patch drift appears, record it as warning and continue
    - if major/minor or probe failures appear, block manual signoff and record details

- [failed] `E2E-061` Deep Check Version can be run when validating a new machine.
  - steps:
    - in `Check Version`, enable `deep run`
    - click `Run Checks`
  - expected:
    - deep probes complete
    - any failure is captured before trusting provider resume/copy behavior on that machine

### Case Group F - Cross-PC Sync To Drive

- [x] `E2E-070` PC1 sync a Codex run to Drive.
  - steps:
    - on PC1, complete `E2E-020`
    - in `History`, find that run row
    - click `Sync`
    - wait for sync success feedback
    - verify the `Remote Chats` section later shows the remote item for the same project
  - expected:
    - sync succeeds without clearing the current timeline
    - no provider session contents are shown in UI/log output

- [ ] `E2E-071` PC1 sync a Claude run to Drive.
  - steps:
    - on PC1, complete `E2E-021`
    - click `Sync` on that run
  - expected:
    - sync result is recorded
    - later restore can be attempted even if provider portability ultimately fails

### Case Group G - Cross-PC Restore From Drive

- [ ] `E2E-080` PC2 restore a synced Codex run.
  - steps:
    - on PC2, connect the same project to the same Drive source
    - open the desktop app
    - select the target project
    - open `Remote Chats`
    - find the synced Codex entry
    - click `Restore`
    - after restore, find the run in `History`
    - click the restored history item
    - send `What token did I ask you to remember?`
  - expected:
    - restore adds one local history item
    - the restored run opens
    - the follow-up succeeds if Codex portability works on PC2

- [ ] `E2E-081` PC2 restore with cwd remap when the original path does not exist.
  - steps:
    - ensure PC1 and PC2 project paths differ
    - sync from PC1
    - on PC2 click `Restore`
    - when the app asks for a project path remap, choose the local project path on PC2
    - retry the restore/open flow
  - expected:
    - restore does not fail silently
    - remap uses the selected PC2 project path
    - the run becomes locally visible afterward

- [ ] `E2E-082` PC2 restore collision handling keeps history valid.
  - steps:
    - on PC2 create or keep an unrelated local run that would collide with the source run id
    - restore the remote run from `Remote Chats`
    - inspect `History`
  - expected:
    - both the unrelated local run and the restored run remain visible
    - restore does not replace unrelated history
    - the restored run can still be opened from its own row

- [ ] `E2E-083` PC2 restore without active provider auth stays greyout-safe.
  - steps:
    - restore a remote run while the target provider account on PC2 is not signed in
    - try to open the restored item
  - expected:
    - the run stays visible
    - the item is disabled with a clear reason
    - no crash and no forced transcript injection occurs

- [ ] `E2E-084` Claude restore result is explicitly classified.
  - expected:
    - record one of:
      - `pass`
      - `greyout-safe`
      - `provider-untested`

### Case Group H - History And Regression Safety

- [ ] `E2E-090` Unified history remains one list with no account grouping.
  - expected:
    - history is still under the single `History` section
    - no new account badge/group/filter appears

- [ ] `E2E-091` Failed resume or failed restore does not erase the current timeline.
  - expected:
    - current timeline remains intact after a typed error

- [ ] `E2E-092` New and restored runs sort by recent activity without breaking older history visibility.

- [ ] `E2E-093` No provider session content appears in logs, UI toasts, tooltips, or error details during any case in this guide.

## 5. Touched Areas

- UI entry points:
  - `Projects`
  - `History`
  - `Remote Chats`
  - `Provider`
  - `Model`
  - `Settings > Check Version`
- runner flows:
  - `POST /client/workflow-runs/{runId}/resume`
  - `POST /client/workflow-runs/{runId}/sync-chat`
  - `POST /client/chat-sessions/restore`
  - `GET /client/projects/{projectId}/workflow-runs`
  - `GET /client/projects/{projectId}/chat-sessions/remote`

## 6. Acceptance Check

- Every case relevant to the environment is marked `pass`, `greyout-safe`, `warning`, or `provider-untested`.
- Codex same-account restart resume is validated.
- Codex cross-account same-PC resume is validated or intentionally recorded as greyout-safe.
- Cross-PC sync and restore are validated on at least one Codex path.
- Check Version confirms the provider portability assumptions before final signoff.

## 7. Out of Scope

- Workflow mode tests.
- Supabase sync tests.
- Browser automation outside the desktop app.
- Full provider auth/connect onboarding instructions.
- Editing provider files beyond the explicit negative tests.

## 8. Completion Notes

- result:
  - This file consolidates the remaining real-provider, real-account, and real-Drive acceptance cases that automated tests do not cover.
  - It is suitable as a manual QA checklist or as an operator script for Computer Use automation.
- follow-ups:
  - Task-072 deferred automated signatures should not be archived if they represent desired future automation; this file only covers the manual/E2E gap.
  - Claude provider portability outcomes should be recorded case-by-case instead of assumed.
