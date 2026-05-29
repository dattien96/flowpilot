## BUG-014: Cannot Kill Claude Process Reliably

### Problem

Today we have multiple places that try to stop AI processes:

- stop `just dev` with `Ctrl+C`
- kill process from workflow-run list page
- kill process from workflow-run detail page
- cleanup on timeout / idle sweep
- cleanup from other background execution flows

The current shell-level approach is not reliable for Claude. We saw cases like:

```text
bash: line 1: kill: (826) - No such process
```

This happens because Claude session execution is not just a simple long-lived PID that the shell can always track safely. Some kill logic runs outside the runner process and therefore does not have the real in-memory session state.

### Root Cause

The wrong layer owns shutdown today.

- Shell / `Justfile` cleanup only knows top-level OS PIDs.
- The local runner owns the real session state.
- Claude session execution can involve per-message child commands that are known inside the runner, not reliably from the shell.
- Restart is impossible if the process that must restart itself is also the one being killed.

So the correct fix is not "improve shell kill more". The correct fix is:

- use runner-native session cleanup to stop active AI sessions
- use a long-lived supervisor for stopping and restarting the web + runner stack

### Required Solution

Implement shutdown/restart in 2 layers.

#### Layer 1: Runner-Native AI Session Cleanup

The local runner must be the source of truth for stopping active AI sessions.

Requirements:

- all active sessions must be closed through runner logic, not direct shell `kill`
- Claude, Gemini, and Codex session cleanup must go through the same runner-owned lifecycle
- timeout / idle sweep cleanup must use the same session-stop path
- workflow-run list and detail page kill actions must keep using runner APIs

Implementation direction:

- keep `CloseSession(...)` and `CleanupSessions()` as the canonical kill path
- ensure every session type can be terminated from runner memory state
- avoid depending on a PID cache as the primary shutdown mechanism
- PID cache may remain only as last-resort fallback, not primary logic

#### Layer 2: Dev Stack Supervisor

`just dev` must act like a supervisor for:

- admin web process
- local runner process

The supervisor must survive long enough to stop and restart both child processes on request.

Requirements:

- when `just dev` starts the stack, it must record enough control metadata in `.flowpilot/`
- supervisor metadata must include at minimum:
  - supervisor PID
  - web PID
  - runner PID
  - control command file or control socket path
- supervisor must accept at least 2 commands:
  - `shutdown`
  - `restart`

### New User Controls

Add 2 UI buttons near the bottom menu area in the admin shell:

- `Shutdown`
- `Restart`

Suggested placement:

- in the left sidebar bottom section
- close to `Appearance` and `Sign out`
- visible when runner is online

### Shutdown Flow

Expected behavior:

1. User starts stack with `just dev`
2. User presses `Shutdown`
3. Admin web calls local runner control endpoint
4. Local runner closes all active AI sessions using runner-native cleanup
5. Local runner signals the supervisor to stop the stack
6. Supervisor stops:
   - admin web
   - local runner
7. UI becomes offline

Important:

- do not kill web/runner first and cleanup later
- AI session cleanup must happen before stack shutdown

### Restart Flow

Expected behavior:

1. User starts stack with `just dev`
2. User presses `Restart`
3. Admin web calls local runner control endpoint
4. Local runner closes all active AI sessions using runner-native cleanup
5. Local runner signals the supervisor to restart the stack
6. Supervisor stops:
   - admin web
   - local runner
7. Supervisor starts them again automatically, equivalent to `just dev`
8. UI health polling detects runner/web are back online

Important:

- restart cannot be implemented by the web app alone
- restart cannot be implemented by the runner alone after it exits
- restart must be owned by the supervisor process created by `just dev`

### API Changes

Add local runner control endpoints for stack lifecycle.

Suggested endpoints:

- `POST /system/shutdown`
- `POST /system/restart`

Behavior:

- validate request
- call runner-native session cleanup first
- delegate stack stop/restart instruction to supervisor
- return accepted/success response quickly

Do not expose direct raw shell command execution from the web.

### UI Changes

In admin web:

- add `Shutdown` button in sidebar bottom controls
- add `Restart` button in sidebar bottom controls
- disable buttons while action is pending
- show clear action state:
  - `Shutting down...`
  - `Restarting...`
- after restart request, poll runner health until online again
- handle offline transition gracefully without noisy error spam

### Justfile / Supervisor Changes

`just dev` must move from simple trap-based cleanup to supervisor-based process management.

Requirements:

- spawn web and runner as supervised child processes
- persist control metadata to `.flowpilot/`
- respond to shutdown/restart commands from runner control layer
- keep `Ctrl+C` behavior working:
  - `Ctrl+C` should still trigger orderly AI session cleanup
  - then stop child processes

Shell trap logic may remain for emergency fallback, but it must not be the primary Claude cleanup strategy.

### Non-Goals

- do not solve this by adding more blind `kill -9`
- do not make browser UI execute `just dev` directly
- do not rely only on cached PIDs for Claude cleanup
- do not duplicate kill logic in many places

### Acceptance Criteria

#### A. Workflow Session Kill

- kill from workflow-run list page stops active Claude sessions correctly
- kill from workflow-run detail page stops active Claude sessions correctly
- same behavior still works for Gemini and Codex sessions

#### B. Timeout / Idle Cleanup

- session timeout cleanup stops Claude sessions correctly
- idle sweeper cleanup stops Claude sessions correctly

#### C. Stack Shutdown

- when stack was started with `just dev`, pressing `Shutdown` stops active AI sessions first
- after cleanup, admin web and local runner both stop
- no orphan Claude process remains

#### D. Stack Restart

- when stack was started with `just dev`, pressing `Restart` stops active AI sessions first
- after cleanup, admin web and local runner both restart automatically
- runner health becomes online again without manual terminal action

#### E. Ctrl+C

- pressing `Ctrl+C` in the terminal running `just dev` still performs orderly cleanup
- Claude sessions are not left orphaned

### Suggested Implementation Order

1. Revert shell-first PID-cache solution
2. Add supervisor model for `just dev`
3. Add runner control endpoints for shutdown/restart
4. Wire runner cleanup into those endpoints
5. Add sidebar `Shutdown` and `Restart` buttons
6. Add health-polling / pending-state UX
7. Verify list/detail/timeout/idle cleanup paths still use runner-native kill flow

### Test Checklist

- start `just dev`
- create active Claude session
- kill from workflow-run list page
- create active Claude session
- kill from workflow-run detail page
- create active Claude session
- wait for timeout / force idle cleanup
- create active Claude session
- press `Shutdown`
- restart stack with `just dev`
- create active Claude session
- press `Restart`
- restart stack with `just dev`
- create active Claude session
- press `Ctrl+C` in terminal

For each case verify:

- session is marked completed correctly when expected
- runner exits cleanly when expected
- web exits cleanly when expected
- no orphan Claude process remains in OS process list
