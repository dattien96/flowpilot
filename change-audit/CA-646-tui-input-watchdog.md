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

### Follow-up 2 (same session, live feedback): always-log fingerprint

Session 9176 wedged at STARTUP — 0 KeyMsg from birth, no flow involved (the
wedge is environmental/intermittent: 21472/25116 ran 5 flows each with 147
keys fine; 25708 wedged at flow start; 9176 wedged at launch). v2's eligibility
gate made that wedge fully invisible (no log, no banner — user only knew the
screen didn't respond). The fingerprint is now **always** logged once per stall
episode in every app state; only the visible banner stays eligibility-gated.

### Follow-up (same session, live feedback): eligibility gate + motion liveness

First live run of the watchdog false-positived on plain idleness: sessions
21472/25580/12300 sat 1-3 minutes without touching keys while the runner was
starting or the screen was being read (`connStatus=connecting`/`idle`,
`flowBlocked=false`) and the banner fired "stalled" even though input was
perfectly fine (each later "recovered" on the next keystroke).

- `inputStallEligible()` — the stall is only flagged when the app is in a
  state that **requires** operator input: `flowLoopBlocked()` (Continue/Stop
  bar — the run-151954 real wedge) or an approval/question/gate card. Silence
  while idle, connecting, or watching a running turn is normal user behavior.
- Outside eligible states the stale stall flag is dropped so a later eligible
  stall re-arms cleanly.
- `lastMotionAt` stamped at the `tuiMsgFilter` level on hover motion (which
  never reaches Update): the stall fingerprint now includes `motionLive=`
  (has the console pipe delivered ANY event during the stall window?), which
  separates "keys dropped upstream" from "console fully dead". Hover motion
  alone never clears a stall — only keys/clicks do.

Deliberately NO mouse-pulse/`EnableMouse` re-issue: CA-610 proved pulsing
drops the next KeyMsg on Windows and bricks input for minutes. The watchdog
only observes and reports; the recovery action (restart / any key) stays with
the operator, and the next occurrence now carries full context in tui.log.

Provider-agnostic (Case 1): the watchdog is pure app-side; no `ProviderKey`
branch.

Will not undo: CA-610 no-pulse invariant, CA-633/636 renderer bounds,
CA-630/631 raw-paste guards.

### Live outcome (08:09, same session)

The recurring "wedges" (sessions 25708/9176/8000) turned out NOT to be a
console bug: the terminal **pane lost input focus** — keyboard/click events
went to the active pane while hover motion (which needs no focus) still
arrived, producing exactly the "pipe alive, no keys" signature. Session 8000:
`motionLive=true` fingerprint → one click inside the TUI screen at 08:08:56 →
keys + F2 arrived instantly (logged `mouse click` + `Update KeyMsg`). The
watchdog's `motionLive` discriminator proved its worth: `motionLive=true` =
focus/active-pane issue (click the pane), `motionLive=false` = true console
death (restart).

## Tests

Additive only — legacy suite untouched.

- `apps/local-runner/internal/tui/app/input_watchdog_test.go` (new):
  - `TestInputWatchdog_StallDetectedOnce` — >45s silence in an input-requiring
    state flags + banner; repeat tick does not re-log/replace.
  - `TestInputWatchdog_StallEligibleViaGateAndQuestion` — approval / question /
    gate cards are all eligible states.
  - `TestInputWatchdog_NotEligibleWhenIdleLogsButNoBanner` — idle silence logs
    the fingerprint but never raises the banner or adds a transcript message.
  - `TestInputWatchdog_EligibleLaterRaisesBannerWithoutRelog` — a stall logged
    while idle raises the banner once an input-requiring state becomes active,
    without a second stall log.
  - `TestInputWatchdog_NoStallBeforeThreshold` — <45s silence changes nothing.
  - `TestInputWatchdog_FirstTickArmsBaseline` — fresh model arms baseline, never
    flags.
  - `TestInputWatchdog_KeyResumesAfterStall` / `...MouseClickResumesAfterStall`
    — arriving input clears flag + banner, refreshes `lastInputAt`.
  - `TestInputWatchdog_MotionStampsLivenessAtFilter` — hover motion stamps
    `lastMotionAt` at the filter, stays dropped, and never clears a stall.
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
summary: TUI input watchdog detects the silent Windows 0-KeyMsg console wedge — 10s tick, lastInputAt stamped on KeyMsg/MouseMsg, one fingerprint log + one visible banner after 45s silence, recovery log on next input (session 25708: 9+ min dead input after flow start); follow-ups: banner only when operator input is required, fingerprint always logged (session 9176 wedged at startup while idle), motionLive= liveness from hover-motion stamping at tuiMsgFilter
# --->8---