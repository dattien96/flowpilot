# CA-671 — BUG-328: stall notice + kill-on-close Job Object for the runner

## Evidence (tui.log pid 12412, 15:08–15:10, FLOWPILOT_DEBUG_VIEW session)

- Renders measured with the per-frame breakdown: `chatPane w=136 h=48
  tuiChrome=0-1.5ms messages=0s paint=0-2.3ms` — the app renders in ~1-2ms,
  the reader never backs up, the queue never fills. The "slow render → queue
  full → terminal wedge" theory (CA-670 phase 2) is disproven.
- ESC/F2/F4 arrived cleanly as VK records (`KeyMsg Type=esc` at 15:09:02 and
  15:09:05), then after the F2/F4/typing burst the host delivered nothing:
  `pendingConsoleEvents=0`, yet a wheel/mouse event still reached Update at
  15:09:11 — **mouse path alive, keyboard path dead**. Same fingerprint across
  5+ sessions and 3 input architectures → the wedge is a Windows Terminal /
  ConPTY keyboard-dispatch bug, not app code. No app-side fix exists; only
  host-side recovery (window close / new pane).

## Changes

1. **Stall notice (input_watchdog.go):** both stall banners now say exactly
   what is true: the terminal stopped delivering keys; **close this terminal
   window to exit — the Go runner is terminated automatically**. Ctrl+C is no
   longer advertised because keyboard delivery (incl. 0x03) is dead during a
   wedge.
2. **Kill-on-close Job Object (runnerboot):** the runner spawned by the TUI is
   now placed in a `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` job. When the TUI
   process dies for ANY reason (force-kill, crash, window close), Windows
   terminates the runner — a wedged TUI can never orphan the `--port` listener.
   Assignment failures are non-fatal (process may already be in a parent job,
   e.g. launched from an IDE) and are logged to `.flowpilot/cli-runner.log`.
3. New test `sysprocattr_job_test.go` (Windows): spawns a child, assigns it to
   the job, closes the handle → child dies (proves the guarantee).
4. Removed the misleading "click terminal for focus" banner copy (the click
   recovery worked in early sessions but is not reliable app guidance).

## Verification

- `go test ./internal/tui/runnerboot/ ./internal/tui/app/ -count=1` green.
- `TestRunnerBootJobKillOnClose` passes (child killed on job close).
- Binary rebuilt 15:26:47.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-328
change_type: bugfix
summary: Stall notice tells user to close window; runner killed via Job Object on any TUI death
# --->8---