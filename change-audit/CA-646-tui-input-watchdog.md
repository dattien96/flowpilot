# CA-646 — TUI input watchdog: detect the silent 0-KeyMsg console wedge

## What

Live session pid 25708 (run-151954, gate-sandbox, rag-harness flow): after the
flow started (06:58:34) the TUI stopped receiving **any** console input — the
tui.log shows the last KeyMsg at 06:58:32, the last mouse click at 06:58:34,
then ZERO input events for 9+ minutes while the user pressed F2 and clicked the
freeze step's [Continue]/[Stop] buttons. The app was NOT dead: the SSE polls
kept delivering `StepsRuntimeMsg` + `agentRunsHydratedMsg` every ~1.6s.

## Why

On Windows the console input path can wedge while the app stays fully alive:
bubbletea v1.3.10 reads conhost input via `PeekNConsoleInputs`/`ReadNConsoleInputs`
(`readConInputs`, key_windows.go); a console-read error kills that goroutine
permanently (0 KeyMsg forever), and a wedged 64-slot ReadConsoleInput queue can
drop everything including key records. The app itself never knows — the poll
loop keeps running, so nothing logged the wedge. CA-633/CA-636 fixed the
RENDERER side of the same symptom class (log 16512/24144/12048); the input
side had no equivalent detector, so the stall stayed silent and undiagnosable.

## Fix

`apps/local-runner/internal/tui/app/input_watchdog.go` (new) + small wiring:

- `cmdInputWatchdog()` — 10s `tea.Tick` re-armed forever from `Init()`.
- `AppModel.lastInputAt` stamped by every `KeyMsg`/`MouseMsg` that reaches
  `Update` (`markInputAlive`, before normal handling).
- `checkInputWatchdog`: when `now - lastInputAt >= 45s`, log ONE diagnostic
  fingerprint with the exact app state at the stall (`connStatus`,
  `flowBlocked`, `viewingChild`, `turnActive`) + show a one-time visible
  banner ("input stalled ... press any key or Ctrl+C restart") so the operator
  knows to restart instead of staring at a dead screen.
- On the next input the stall flag clears, the banner is removed, and a
  recovery line is logged (`input-watchdog: console input recovered ...`).
- `inputWatchdogMsg` is excluded from the generic Update log (no 10s log
  noise).

Deliberately NO mouse-pulse/`EnableMouse` re-issue: CA-610 proved pulsing
drops the next KeyMsg on Windows and bricks input for minutes. The watchdog
only observes and reports; the recovery action (restart / any key) stays with
the operator, and the next occurrence now carries full context in tui.log.

Provider-agnostic (Case 1): the watchdog is pure app-side; no `ProviderKey`
branch.

Will not undo: CA-610 no-pulse invariant, CA-633/636 renderer bounds,
CA-630/631 raw-paste guards.

## Tests

Additive only — legacy suite untouched.

- `apps/local-runner/internal/tui/app/input_watchdog_test.go` (new):
  - `TestInputWatchdog_StallDetectedOnce` — >45s silence flags + banner; repeat
    tick does not re-log/replace.
  - `TestInputWatchdog_NoStallBeforeThreshold` — <45s silence changes nothing.
  - `TestInputWatchdog_FirstTickArmsBaseline` — fresh model arms baseline, never
    flags.
  - `TestInputWatchdog_KeyResumesAfterStall` / `...MouseClickResumesAfterStall`
    — arriving input clears flag + banner, refreshes `lastInputAt`.
  - `TestInputWatchdog_CmdReArms` — watchdog tick always returns a cmd.
  - `TestInputWatchdog_NormalKeyDoesNotTouchBanner` — a normal key never
    clobbers a legitimate live status banner.

## Verification

- `go test ./internal/tui/app/ -count=1` → green (full suite, legacy + new).
- `go build ./internal/tui/...` clean; `go vet ./internal/tui/...` clean.

## Residual

The exact wedge mechanism (reader goroutine death vs conhost queue) is not yet
pinned from static analysis — it needs a live repro. The watchdog's fingerprint
log is designed to capture it: the next stalled session should be diagnosed
from the `input-watchdog: console input stalled ... connStatus=... flowBlocked=...`
line, and if it recurs the fingerprint goes into a follow-up fix.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-646
change_type: bugfix
summary: TUI input watchdog detects the silent Windows 0-KeyMsg console wedge — 10s tick, lastInputAt stamped on KeyMsg/MouseMsg, one fingerprint log + one visible banner after 45s silence, recovery log on next input (session 25708: 9+ min dead input after flow start)
# --->8---