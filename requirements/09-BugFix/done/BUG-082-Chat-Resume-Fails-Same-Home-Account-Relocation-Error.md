---
name: BUG-082-Chat-Resume-Fails-Same-Home-Account-Relocation-Error
description: Clicking a local history chat returned "could not prepare session" and the item went disabled then reset, because RelocateSessionFile incorrectly treated same-path src/dst as a collision.
metadata:
  type: bugfix
---

## Metadata

- Document ID: `BUG-082`
- Title: Chat Resume Fails — Same-Home Account Triggers Relocation Collision
- Phase: `bugfix`
- Status: `done`
- Owner: DatNguyen
- Reviewers: —
- Created: 2026-06-18
- Last Updated: 2026-06-18
- Parent Documents: `Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md`
- Child Documents: —
- Related Documents: `BUG-074-Desktop-History-Replay-Leaves-Resolved-Approvals-Pending.md`, `BUG-080-Desktop-Run-History-Lost-On-App-Restart-No-Supabase.md`
- Replaces: —
- Tags: resume, cross-account, single-account, regression, severity-high

## AI Quick View

### Summary

- Clicking any local history chat produced the tooltip "could not prepare the session on the active account", disabled the row, then auto-recovered after the 10 s poll cycle.
- Root cause: `prepareCrossAccountResume` is called when `rs.providerAccountID != activeAccountForProvider()`. On a single-account machine (or when two account entries share the same home directory), `relocationTargetPath` returns the same path for source and destination. `RelocateSessionFile` then hit the "destination already exists" guard and returned an error, surfacing as `session_unavailable`.
- `providerAccountID` mismatch arises when the active account re-uses the default home path under a new ID (e.g. after `[BugFix]: resolve active account per provider` commit `20e6dc1` stamped runs with the real account ID, while older in-memory logic used `"default"`), or when the user switches between two accounts mapped to the same home.
- `loadRunHistory` polling clears `unavailableReason` every 10 s, making the chat appear to "recover" but still fail on the next click.

### Current Ask

- Make `RelocateSessionFile` a no-op when src and dst are the same file; let `prepareCrossAccountResume` rebind and persist the new account ID so future resumes use the same-account fast path.

### Key Decisions

- `V-1` `TestRelocateSessionFileSameHomeCodexIsNoop` / `TestRelocateSessionFileSameHomeClaudeIsNoop` — same home must return `(srcPath, nil)`, no copy.
- `V-2` `TestResumeRunSameHomeAccountRebindsProviderAccountID` — after same-home resume, persisted `ProviderAccountID` must equal the active account ID.
- `V-3` Existing `TestRelocateSessionFileDoesNotOverwriteExistingSessionFile` must continue to pass (different-home overwrite protection unchanged).

### Constraints

- Do not change the overwrite-protection behaviour for genuinely cross-home relocations.
- Fix must be limited to `session_file_locator.go` + new tests; `prepareCrossAccountResume` logic is unchanged.

### Open Questions

- None.

### Source Refs

- User screenshot: sidebar showing all chats greyed out with sync count drop from 3 → 2.
- Error path: `interactive_resume.go:149` → `session_file_locator.go:70–72` ("destination session file already exists").
- Commit `20e6dc1` introduced `activeAccountForProvider` which can return a real account ID that differs from the `"default"` stored in old runs, triggering cross-account resume unnecessarily on same-home machines.

## 1. Issue Summary

After clicking a local history chat, the item briefly appeared disabled with the tooltip "could not prepare the session on the active account". Ten seconds later the history poll refreshed and the item became clickable again, but clicking it produced the same failure.

## 2. Parent Links

- impacted coding plan: `CP-18-Refactor-Workflow-With_Session.md`
- impacted tech design: `Task-067-Desktop-Post-Restart-Run-Resume-Via-Provider-Session-Id.md`
- impacted system spec: —

## 3. Environment and Reproduction

- environment: Desktop Electron app, any OS; single-account machine or two provider accounts sharing the same home directory
- reproduction steps:
  1. Create one or more chat runs
  2. The active provider account's ID differs from `rs.providerAccountID` stored in the run (e.g. app updated, or default account re-registered)
  3. Click any completed chat in the sidebar → "could not prepare session…" tooltip, item disabled, resets after 10 s
- frequency: 100% on affected account configurations

## 4. Expected vs Actual

- expected: Clicking a history chat opens the transcript successfully
- actual: Chat fails to open with `session_unavailable`, row briefly disabled, then resets

## 5. Impact

- users affected: Any user whose active account ID differs from the ID stamped on old runs AND both map to the same home directory
- workflows affected: Chat history replay / resume
- severity: High — all local chats are inaccessible until the account ID is corrected

## 6. Root Cause

- hypothesis: `relocationTargetPath` produces `dstPath == srcPath` when `targetHome == srcHome`
- confirmed cause: `RelocateSessionFile` guards against overwrite with `os.Stat(dstPath); err == nil → return error`. When both accounts share the same home directory, `dstPath == srcPath` (the file is already at its intended location), so the guard fires incorrectly.
- evidence: Tracing `relocationTargetPath` for Codex (`{home}/sessions/…/rollout-xyz.jsonl` → same path when targetHome == srcHome) and Claude (`{home}/.claude/projects/{hash}/{id}.jsonl` → same path). Confirmed by three new unit tests.

## 7. Fix Strategy

- `F-1` In `RelocateSessionFile` (`session_file_locator.go`): after computing `dstPath`, check `filepath.Clean(dstPath) == filepath.Clean(srcPath)`. If equal, return `(srcPath, nil)` immediately — the file is already in place.
- `F-2` No changes needed in `prepareCrossAccountResume`: on success it already sets `rs.providerAccountID = activeAccountID` and calls `persistProviderSession`, rebinding the stored ID to the active account for all future resumes.

## 8. Validation

- `V-1` `TestRelocateSessionFileSameHomeCodexIsNoop` — Codex same-home returns `(srcPath, nil)` ✅
- `V-2` `TestRelocateSessionFileSameHomeClaudeIsNoop` — Claude same-home returns `(srcPath, nil)` ✅
- `V-3` `TestResumeRunSameHomeAccountRebindsProviderAccountID` — full resume path: succeeds and persists `ProviderAccountID = "acct-new"` ✅
- `V-4` All 11 existing `TestRelocateSession*` / `TestPrepareCrossAccount*` tests pass unchanged ✅

## 9. Regression Guard

- tests: 3 new tests + all prior relocation/cross-account tests green
- alerts: —
- audit checks: `RelocateSessionFile` callers are `prepareCrossAccountResume` and the session-sync route; both remain correct.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — this is a locator-function fix, not a design rule change.
- notes left unchanged on purpose: The "destination already exists" guard for genuinely different-home accounts is intentionally preserved to prevent data clobbering.
