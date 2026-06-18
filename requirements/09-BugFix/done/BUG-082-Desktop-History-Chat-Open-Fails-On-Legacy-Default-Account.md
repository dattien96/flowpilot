---
name: BUG-082-Desktop-History-Chat-Open-Fails-On-Legacy-Default-Account
description: Reopening local history chats greyed them out with "could not prepare the session on the active account" because RelocateSessionFile treated same-path src/dst as a collision when a legacy "default" account ID no longer matched the active durable account ID.
metadata:
  type: bugfix
---

## Metadata

- Document ID: `BUG-082`
- Title: Desktop History Chat Open Fails On Legacy Default Account
- Phase: `bugfix`
- Status: `done`
- Owner: DatNguyen
- Reviewers: —
- Created: 2026-06-18
- Last Updated: 2026-06-18
- Parent Documents: [Task-071: Cross-Account Chat Resume DOD Checklist](../../08-Task/todo/Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md), [Task-075: Cross-Account And Cross-PC Chat E2E Test Guide](../../08-Task/todo/Task-075-Cross-Account-And-Cross-PC-Chat-E2E-Test-Guide.md)
- Child Documents: —
- Related Documents: [BUG-074-Desktop-History-Replay-Leaves-Resolved-Approvals-Pending.md](./BUG-074-Desktop-History-Replay-Leaves-Resolved-Approvals-Pending.md), [BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md](./BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md), [CA-099: Fix Desktop Legacy Default Account Chat Resume](../../../change-audit/CA-099-fix-desktop-legacy-default-account-chat-resume.md)
- Replaces: —
- Tags: desktop, history, local-runner, resume, provider-account, regression, severity-high

## AI Quick View

### Summary

- Clicking a local history chat briefly disabled the row with "could not prepare the session on the active account", then auto-recovered after the 10 s poll cycle — making the chat permanently inaccessible by click.
- Rows were persisted with `provider_account_id: "default"` before provider accounts received durable generated IDs; after account sync the active account has a real ID (e.g. `aaa4752…` for Codex, `c0220…` for Claude) that points to the same physical home directory.
- `ensureResumeReady` sees `"default" != "aaa4752…"` and calls `prepareCrossAccountResume`; `RelocateSessionFile` then computes `dstPath == srcPath` (same home) and hits the overwrite-protection guard, returning "destination session file already exists" → `session_unavailable`.
- Fix: `RelocateSessionFile` now returns `(srcPath, nil)` when `dstPath == srcPath`; `prepareCrossAccountResume` proceeds to rebind and persist `providerAccountID → activeAccountID` so all future resumes go through the same-account fast path.

### Current Ask

- Find and fix why completed/cancelled local desktop chats cannot be opened from the left history list.

### Key Decisions

- `V-1` Treat same-home source/destination as a successful no-op relocation, not an overwrite collision.
- `V-2` Keep the overwrite-protection guard for genuinely different-home cross-account/cross-PC destinations.
- `V-3` Let `prepareCrossAccountResume` rebind and persist the new account ID on the no-op path so the bug self-heals on first successful open.

### Constraints

- Do not overwrite provider session files for true cross-account destinations.
- Keep legacy `sessions.ndjson` rows loadable without migration.
- Preserve the unified history list with no provider-account grouping in the UI.

### Open Questions

- None.

### Source Refs

- User screenshots 2026-06-18 showing all sidebar history rows greyed out after click.
- Local `.flowpilot/chats/sessions.ndjson` rows with `provider_account_id: "default"`.
- `C:\Users\dat.nguyen\AppData\Roaming\FlowPilot\provider-accounts.json` with active durable IDs `aaa4752…` (Codex) and `c0220…` (Claude), both pointing to the default provider homes.
- `apps/local-runner/internal/runner/interactive_resume.go`
- `apps/local-runner/internal/runner/session_file_locator.go`
- `apps/local-runner/internal/runner/cross_account_resume_test.go`
- GitNexus impact: `RelocateSessionFile` — LOW risk, 4 upstream callers, runner module only, 0 affected processes.

## 1. Issue Summary

Clicking any local history chat (Codex or Claude) from the desktop left sidebar produced the tooltip "could not prepare the session on the active account". The row became disabled, and the main view returned to the initial state without opening the transcript. Ten seconds later `loadRunHistory` polling refreshed the history, clearing `unavailableReason`, and the row became clickable again — only to fail identically on the next click.

## 2. Parent Links

- impacted coding plan: `CP-18-Refactor-Workflow-With_Session.md`
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: Desktop Electron app, local runner, local `.flowpilot/chats/sessions.ndjson` history, provider account registry enabled
- reproduction steps:
  1. Have a history row persisted with `provider_account_id: "default"` (created before or during provider-account ID migration).
  2. Provider account sync creates durable active account IDs (e.g. `aaa4752…` for Codex) pointing to the same physical default home.
  3. Click the history row in the desktop sidebar.
- frequency: 100% reproducible for affected legacy rows on the inspected machine

## 4. Expected vs Actual

- expected: Chat opens using the existing provider session file, transcript replays, and run can continue.
- actual: `resumeRun` returns `session_unavailable: "could not prepare the session on the active account"`; desktop stores it in `unavailableReason`, disables the row, leaves the main view unchanged.

## 5. Impact

- users affected: Desktop users with local chat history created before provider-account IDs became durable (i.e. before commit `20e6dc1` or during the transition window).
- workflows affected: Reopening completed/cancelled local Codex or Claude chats from history after app restart or account sync.
- severity: High — existing chats appear in the list but cannot be opened.

## 6. Root Cause

- hypothesis: `RelocateSessionFile` incorrectly fails when `dstPath == srcPath`.
- confirmed cause:
  1. `openHistoryRun` calls `client.resumeRun(runId)`.
  2. Runner reconstructs the run from `sessions.ndjson` with `providerAccountID = "default"`.
  3. `ensureResumeReady` calls `activeAccountForProvider(providerKey)` which returns the durable active ID (e.g. `"aaa4752…"`).
  4. `"default" != "aaa4752…"` → `prepareCrossAccountResume` is called.
  5. `srcHome = resolveAccountHome("default")` → `DetectDefaultAccountHomePath` → e.g. `C:\Users\dat\.codex`.
  6. `targetHome = resolveAccountHome("aaa4752…")` → account's `HomePath` from config → same `C:\Users\dat\.codex`.
  7. `relocationTargetPath` for Codex splits on `/sessions/` and rejoins under `targetHome` → produces the identical absolute path.
  8. `os.Stat(dstPath)` succeeds (file exists) → `return "", errors.New("destination session file already exists")`.
  9. `prepareCrossAccountResume` maps the error to `session_unavailable: "could not prepare the session on the active account"`.
- evidence: Actual `provider-accounts.json` on user's machine confirms both `"default"` (legacy) and `"aaa4752…"` (active) map to the same Codex home. Same pattern confirmed for Claude (`c0220…`). Three new unit tests reproduce and verify the fix.

## 7. Fix Strategy

- `F-1` In `RelocateSessionFile` (`session_file_locator.go`): after computing `dstPath`, check `filepath.Clean(dstPath) == filepath.Clean(srcPath)`. If equal, return `(srcPath, nil)` immediately — the session file is already in place, nothing to copy.
- `F-2` Keep the existing `os.Stat` overwrite guard unchanged for cases where `dstPath != srcPath` (genuine cross-home destinations must not be overwritten).
- `F-3` No change to `prepareCrossAccountResume`: on the `(srcPath, nil)` success path it already sets `rs.providerAccountID = activeAccountID` and calls `persistProviderSession`, rebinding the stored ID for all future resumes.

## 8. Validation

- `V-1` `TestRelocateSessionFileSameHomeCodexIsNoop` — Codex same-home returns `(srcPath, nil)`, no copy ✅
- `V-2` `TestRelocateSessionFileSameHomeClaudeIsNoop` — Claude same-home returns `(srcPath, nil)`, no copy ✅
- `V-3` `TestResumeRunSameHomeAccountRebindsProviderAccountID` — full resume succeeds and persisted `ProviderAccountID` equals `"acct-new"` ✅
- `V-4` `TestRelocateSessionFileDoesNotOverwriteExistingSessionFile` — different-home overwrite still refused ✅
- `V-5` `TestPrepareCrossAccountResumeRelocatesAndRepointsRun` — true cross-home relocation still works ✅
- `V-6` `TestPrepareCrossAccountResumeRelocationFailureKeepsHistoryVisible` — different-home pre-existing dst still returns `session_unavailable` ✅
- `V-7` GitNexus impact: `RelocateSessionFile` LOW risk; `prepareCrossAccountResume` / `ensureResumeReady` LOW risk, 0 affected processes.

## 9. Regression Guard

- tests: 3 new tests + 8 prior relocation/cross-account tests, all green.
- alerts: —
- audit checks: True relocation failures (missing auth, missing file, genuine destination collision) remain greyout-safe. Only the false same-home collision is corrected.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — the fix preserves Task-071's mutable provider-account pointer model.
- notes left unchanged on purpose: Task-075 manual E2E checklist still covers broader same-account/cross-account manual cases not replaced by this fix.
