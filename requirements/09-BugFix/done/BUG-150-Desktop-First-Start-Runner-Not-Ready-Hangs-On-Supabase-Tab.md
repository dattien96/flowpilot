# BUG-150: Desktop First Start Runner Not Ready Hangs On Supabase Tab

## Metadata

- Document ID: `BUG-150`
- Title: `Desktop First Start Runner Not Ready Hangs On Supabase Tab`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-30`
- Last Updated: `2026-06-30`
- Parent Documents: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- Child Documents: `none`
- Related Documents: `BUG-053-Desktop-Settings-Nav-Order-And-Runner-Status-Cue.md`, `change-audit/CA-145-desktop-first-start-runner-retry.md`
- Replaces: `none`
- Tags: `desktop-flowpilot, runner, startup, race-condition, settings, supabase`

## AI Quick View

### Summary

- On the first `just dev` start the Go runner binary must compile before it can serve; the desktop Electron/Vite app starts faster and fires its bootstrap probe before the runner port is open.
- `loadSupabaseRuntimeStatus` catches the resulting ECONNREFUSED and returns `runnerReachable: false`, but there was no retry loop — the app stayed permanently on the Settings/Supabase screen.
- The unauthenticated `SettingsShell` was rendered with `visibleSections={["supabase"]}` which excluded the Runner tab, so even when `preferredSettingsSection` resolved to `"runner"` it was silently overridden to `"supabase"`.
- The `http:request` IPC handler in Electron's main process had no timeout, so any request that hit a TCP-listening-but-not-yet-serving runner would hang indefinitely.

### Current Ask

- Fix the three root causes so the desktop automatically waits for the runner on first start.

### Key Decisions

- `V-1` Retry bootstrap up to 15 × 2 s (30 s total) before presenting a terminal failure state.
- `V-2` Show the Runner tab in unauthenticated Settings when runner is offline so the user gets actionable status.
- `V-3` Abort IPC fetch after 8 s to prevent indefinite hangs in edge cases.

### Constraints

- The retry loop must not affect subsequent calls to `refreshBootstrap` (login, save-Supabase) — those calls already succeed on the first attempt because the runner is up by then.
- The 8 s IPC abort must not interfere with normal streaming endpoints that legitimately take longer; `http:request` is used only for short JSON round-trips.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/electron/main.ts` IPC handler `http:request`
- `apps/desktop-flowpilot/src/App.tsx` `refreshBootstrap` and unauthenticated `SettingsShell`
- `apps/local-runner/internal/cli/root.go` — runner calls `http.ListenAndServe` after all initialization (line ~1658)

## 1. Issue Summary

When `just dev` is run for the first time the desktop app opens and shows the Supabase settings tab and then appears to hang with no visible progress. Closing and restarting with `just dev` a second time works correctly because the runner binary is already compiled and starts within seconds.

The root cause is a three-way combination:
1. No retry on runner-unreachable in the bootstrap flow.
2. The unauthenticated settings shell hides the Runner tab (`visibleSections={["supabase"]}`), masking the true offline state.
3. No timeout on the IPC HTTP fetch in Electron's main process.

## 2. Parent Links

- impacted coding plan: none (infrastructure startup behaviour, not a Coding Plan feature)
- impacted tech design: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- impacted system spec: none

## 3. Environment and Reproduction

- environment: Windows 11, `just dev` (supervisor.js with `--restart-existing --with-desktop`)
- reproduction steps:
  1. Ensure runner is not currently running.
  2. Run `just dev` for the first time (no cached Go build).
  3. Observe desktop app opens and stays on Settings → Supabase tab.
  4. App never transitions; runner eventually starts but desktop does not re-probe.
- frequency: 100 % on first start; absent on second start (runner already compiled and running).

## 4. Expected vs Actual

- expected: Desktop shows "Bootstrapping desktop workspace…" spinner, polls until runner is ready (≤ 30 s), then transitions to the correct post-boot screen.
- actual: Desktop fires a single bootstrap probe, gets ECONNREFUSED, silently falls back to Settings → Supabase, and never re-checks runner availability.

## 5. Impact

- users affected: all developers using `just dev` on a clean checkout or after clearing the Go build cache.
- workflows affected: first-time desktop setup; any new-machine developer onboarding.
- severity: medium (workaround is a second `just dev` run; no data loss).

## 6. Root Cause

- hypothesis: Race condition between slow Go compilation and fast Electron startup.
- confirmed cause:
  1. `refreshBootstrap` in `App.tsx` did not retry when `runnerReachable === false`.
  2. `SettingsShell` `visibleSections={["supabase"]}` hid the Runner tab, so `preferredSettingsSection: "runner"` was overridden to `"supabase"` by the fallback in `SettingsShell.tsx` line 78.
  3. The `http:request` IPC handler in `main.ts` called `fetch()` with no timeout, meaning any request to a port that had opened at the OS level but was not yet handled by Go's HTTP server would hang indefinitely.
- evidence:
  - `go run ./cmd/flowpilot runner serve` compiles before starting; first run with cold cache can take 15-30 s.
  - `loadSupabaseRuntimeStatus` wraps all fetch errors in a `try/catch` returning `demoStatus(false, ...)`, so the app never enters the `catch` in `refreshBootstrap` — it always calls `applyBootstrapState`, which routes to `"runner"` section. But that section is not in `visibleSections`, so `SettingsShell` shows `"supabase"` instead.

## 7. Fix Strategy

- `F-1` **`apps/desktop-flowpilot/electron/main.ts`**: Add `AbortController` with 8 s timeout to the `http:request` IPC handler's `fetch()` call.
- `F-2` **`apps/desktop-flowpilot/src/App.tsx` `refreshBootstrap`**: Replace single-shot bootstrap probe with a retry loop (15 attempts × 2 s delay = 30 s max). Retry while `runtimeStatus.runnerReachable === false`; on final attempt (or catch) call `applyBootstrapState` / set error state.
- `F-3` **`apps/desktop-flowpilot/src/App.tsx` unauthenticated `SettingsShell`**: Change `visibleSections` from the static `["supabase"]` to `runtimeStatus.runnerReachable ? ["supabase"] : ["supabase", "runner"]` so the Runner health panel is reachable when the runner is offline.

## 8. Validation

- `V-1` TypeScript `tsc --noEmit` passes with no errors in `apps/desktop-flowpilot` after the changes.
- `V-2` Manual: run `just dev` from a clean state (no cached runner); observe the loading spinner for up to 30 s before the app transitions to the correct screen (Supabase tab if runner up, Runner tab if still offline after retries).
- `V-3` Manual: run `just dev` a second time when runner is already listening; confirm bootstrap succeeds on the first attempt with no observable delay.

## 9. Regression Guard

- tests: no automated test added — the race involves real OS process timing and Electron IPC; integration test would require a process supervisor harness.
- alerts: none.
- audit checks: `CA-145-desktop-first-start-runner-retry.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this is a pure implementation deficiency, not a design decision change.
- notes left unchanged on purpose: `SD-03` and `R3-Phase1-Checklist` do not describe the bootstrap retry contract; no update needed.
