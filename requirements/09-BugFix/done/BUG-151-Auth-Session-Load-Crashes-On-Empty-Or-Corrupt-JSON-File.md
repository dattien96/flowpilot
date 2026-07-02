# BUG-151: Auth Session Load Crashes On Empty Or Corrupt JSON File

## Metadata

- Document ID: `BUG-151`
- Title: `Auth Session Load Crashes On Empty Or Corrupt JSON File`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-30`
- Last Updated: `2026-06-30`
- Parent Documents: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- Child Documents: `none`
- Related Documents: `BUG-150-Desktop-First-Start-Runner-Not-Ready-Hangs-On-Supabase-Tab.md`, `change-audit/CA-146-auth-session-corrupt-file-recovery.md`
- Replaces: `none`
- Tags: `desktop-flowpilot, auth, ipc, regression, electron`

## AI Quick View

### Summary

- `loadPersistedAuthSession` in `electron/main.ts` reads `supabase-auth-session.json` and calls `JSON.parse` on its contents.
- The catch block only swallowed `ENOENT` (file not found); any other error, including `SyntaxError` from an empty or truncated file, was re-thrown and surfaced to the renderer as `Error invoking remote method 'auth-session:load': SyntaxError: Unexpected end of JSON input`.
- `savePersistedAuthSession` used a non-atomic `writeFile`, so a process kill between truncation and write left a zero-byte file on disk.

### Current Ask

- Handle empty/corrupt session files gracefully and prevent corruption with an atomic write.

### Key Decisions

- `V-1` A `SyntaxError` from `JSON.parse` is treated as "no session" and the corrupt file is deleted so subsequent saves work normally.
- `V-2` `savePersistedAuthSession` writes to a `.tmp` file then `rename`s to the target, making the write atomic on NTFS and POSIX file systems.

### Constraints

- The fix must not silently discard a valid session — only truly unparseable files are deleted.
- `rename` is safe on NTFS because source and destination are on the same volume (same `userData` directory).

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/electron/main.ts` — `loadPersistedAuthSession`, `savePersistedAuthSession`

## 1. Issue Summary

Opening the desktop app after a previous run that was forcibly killed (e.g. `just dev --restart-existing`) can leave `supabase-auth-session.json` as a zero-byte file. On next startup, `readFile` succeeds (the file exists), but `JSON.parse("")` throws `SyntaxError: Unexpected end of JSON input`. The catch block only handled `ENOENT`, so the error propagated through the IPC boundary and was logged as `Error invoking remote method 'auth-session:load': SyntaxError: Unexpected end of JSON input`.

## 2. Parent Links

- impacted coding plan: none
- impacted tech design: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- impacted system spec: none

## 3. Environment and Reproduction

- environment: Windows 11, desktop-flowpilot Electron app, any `just dev` restart that kills the process mid-write.
- reproduction steps:
  1. Log in to the desktop app so `supabase-auth-session.json` is written.
  2. Kill the Electron process while it is in the middle of writing the file (race; easier to reproduce by truncating the file manually: `echo "" > %APPDATA%\FlowPilot\supabase-auth-session.json`).
  3. Restart the desktop app.
  4. Observe `Error invoking remote method 'auth-session:load': SyntaxError: Unexpected end of JSON input` in the DevTools console.
- frequency: intermittent; more likely after `just dev --restart-existing` which sends SIGINT to a running process.

## 4. Expected vs Actual

- expected: Corrupt/empty session file is treated as "no session"; app starts normally and the user is prompted to log in.
- actual: IPC handler throws, renderer receives an unhandled rejection, and the app may show an error or hang at the auth step.

## 5. Impact

- users affected: developers using `just dev --restart-existing` who were previously logged in.
- workflows affected: desktop auth / bootstrap flow.
- severity: low-medium (workaround is to delete `supabase-auth-session.json` from `%APPDATA%\FlowPilot`).

## 6. Root Cause

- hypothesis: Non-atomic write leaves zero-byte file after a mid-write process kill.
- confirmed cause:
  1. `savePersistedAuthSession` called `writeFile` (which truncates first, then writes). A kill signal between truncation and write produces a zero-byte file.
  2. `loadPersistedAuthSession` catch only handled `ENOENT`; `SyntaxError` from `JSON.parse("")` was re-thrown.
- evidence: Error message `SyntaxError: Unexpected end of JSON input` is the exact output of `JSON.parse("")` in V8.

## 7. Fix Strategy

- `F-1` In `loadPersistedAuthSession`: add `if (error instanceof SyntaxError) { await rm(..., { force: true }); return null; }` before the final `throw`.
- `F-2` In `savePersistedAuthSession`: write to `dest + ".tmp"` via `writeFile`, then `rename(tmp, dest)`. Add `rename` to the `node:fs/promises` import.

## 8. Validation

- `V-1` TypeScript `tsc --noEmit` passes with no errors.
- `V-2` Manual: truncate `supabase-auth-session.json` to 0 bytes, restart desktop; confirm app starts normally without IPC error and the file is deleted.
- `V-3` Manual: log in normally; confirm session is persisted and loads correctly on next start.

## 9. Regression Guard

- tests: no automated Electron IPC test added; manual regression check sufficient for a file-read guard.
- alerts: none.
- audit checks: `CA-146-auth-session-corrupt-file-recovery.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: the `PersistedAuthSession` schema and the broader auth flow are unchanged.
