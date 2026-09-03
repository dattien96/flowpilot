# BUG-328: TUI input dies under Windows mouse tracking; replace click chips with a keyboard action ring

## Metadata

- Document ID: `BUG-328`
- Title: `TUI input dies under Windows mouse tracking; replace click chips with a keyboard action ring`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-28`
- Last Updated: `2026-09-02`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/done/CP-56-Terminal-TUI-Chat-And-Flow-Client.md)
- Child Documents: `none`
- Related Documents: [CA-646](../../../change-audit/CA-646-tui-input-watchdog.md), [CA-636](../../../change-audit/CA-636-bound-renderer-output-console-write-block.md), [CA-633](../../../change-audit/CA-633-tui-idle-compose-cache-and-sidebar-collapsed.md), [CA-631](../../../change-audit/CA-631-restore-production-raw-paste-reject-guard.md), [CA-621](../../../change-audit/CA-621-tui-open-view-key-drop.md), [CA-615](../../../change-audit/CA-615-tui-altv-clipboard-timeout.md), [CA-612](../../../change-audit/CA-612-tui-ctrlv-no-stream.md), [CA-610](../../../change-audit/CA-610-tui-input-live-after-mouse-paste.md), [CA-653](../../../change-audit/CA-653-tui-attach-open-without-cmd-focus.md), [CA-535](../../../change-audit/CA-535-tui-input-freeze-provider-probes.md), [BUG-325](../done/BUG-325-TUI-Chat-Freezes-After-Flow-Gate-With-Empty-Options.md)
- Replaces: `none`
- Tags: `cli-tui, windows, console-input, hang, keyboard-ux, mouse-tracking, severity-high`

## AI Quick View

### Summary

- On Windows, `WithMouseCellMotion` puts the console into application mouse mode (`ENABLE_MOUSE_INPUT` + CSI `?1002h`). The host then delivers hover (and sometimes clicks) to the PTY **without keyboard focus**, so `KeyMsg` stops for minutes while the event loop stays alive.
- Live proof: pid `17672` received `mouse click` at `10:19:24` then zero keys for 2 minutes; pid `4900` typed `/pr` then went silent `10:34:56`–`10:35:42` while `sessionLoadTimeoutMsg` still ran; operator clicks during hangs often never appear in `tui.log`.
- Cursor CLI (Ink) and default Bubble Tea **do not** enable mouse tracking. Claude Code documents the same footgun and ships `CLAUDE_CODE_DISABLE_MOUSE_CLICKS`. FlowPilot must follow that pattern, not Win32 self-heal.
- Click chips stay on screen as labels. Interaction moves to a keyboard action ring (highlight + arrows + Enter), matching `/mode-setup` and other TUIs. Slash commands remain fallback, not the primary path.

### Current Ask

- Stop requesting cell-motion mouse from the host (all platforms, so UX matches).
- Give every former click target a keyboard path that is **selection + Enter**, not typing `/command`.
- Keep F2/F3/F4, Tab agent-cycle, `/stop`/`/continue`, Alt+V, `/copy`, and native terminal selection.

### Key Decisions

- `D-1` Remove `tea.WithMouseCellMotion()` from `tuiProgramOpts`. Keep `WithAltScreen` and `tuiMsgFilter` (filter becomes a no-op for mouse). Do not pulse Enable/DisableMouse (CA-610).
- `D-2` Keyboard is the only chip interaction. Hit-test code may remain for a later opt-in; it must not be required to complete any flow.
- `D-3` Action ring uses **Left/Right + Enter**. Tab is **not** stolen — it keeps slash picker, then agent cycle (`cmdFocusAgent` = `[open]`), then posture.
- `D-4` Two different Stops: live turn = `Ctrl+C` or `/stop`; parked flow = highlighted `[Stop]` + Enter, or `/stop`. `Ctrl+C` with empty input and no live turn still quits (unchanged).
- `D-5` No `SetForegroundWindow`, no in-place `tea.NewProgram` restart, no `GetConsoleWindow` foreground gate (wrong on Windows Terminal / ConPTY).
- `D-6` Copy chips are dropped. Native bôi-đen of the host is the copy path once mouse tracking is off; `/copy` stays.

### Constraints

- additive-tests-only for pre-existing files. New tests cover the ring and `tuiProgramOpts`. Do not edit `input_watchdog_test.go` without approval (`Q-2`).
- Must not undo: CA-610 no-pulse, CA-612/631 paste guards, CA-615 clipboard timeouts, CA-621 log throttle, CA-633 idle cache, CA-636 queued output, CA-535/CA-653 console-detach for children.
- **Will** change `handleKey` and chip rendering (highlight). Will not change paste burst, textarea core, or renderer bounds.
- Provider-agnostic: no `ProviderKey` branch.
- Same keyboard contract on Windows, macOS, and Linux.

### Open Questions

- `Q-1` Closed. Mechanism is host mouse-tracking vs keyboard focus, not a dead Bubble Tea reader (`motionLive=true` and click-without-keys both observed).
- `Q-2` `TestInputWatchdog_EligibleLaterRaisesBannerWithoutRelog` asserts `statusMsg != ""`, already true from `New()` seeding `"connecting..."`. Fixing it edits a pre-existing test — needs explicit approval. Out of this bug’s keyboard scope.
- `Q-3` Cancelled. In-place program restart is not in this fix.

### Source Refs

- `%APPDATA%\FlowPilot\tui.log` 2026-08-28: pid `17672` (`10:19:24` click, 2 min no keys), pid `4900` (`10:34:56`–`10:35:42` gap), pid `10068`/`6708` (`motionLive=true`, zero keys from start).
- Cursor CLI 2026.08.25 (`cursor-agent`): Ink `setRawMode`, bracketed paste `?2004h` only, no `?1002h`/`?1003h`; composer is `useInput`.
- Claude Code: `CLAUDE_CODE_DISABLE_MOUSE_CLICKS` — “there is no click that only focuses the window”.
- Code: `tuiProgramOpts`, `handleKey`, `mouse.go` `clickTargetAt`/`dispatchMouseClick`, `mode_setup_modal.go` (ring precedent), `input_watchdog.go`.

## 1. Issue Summary

`flowpilot chat` on Windows stops taking keyboard (and often click) input while the process is alive. Enabling application mouse mode is the in-app cause we can remove. Replacing chips with slash commands is not acceptable UX; the replacement is a highlighted action ring.

## 2. Parent Links

- impacted coding plan: CP-56 (Terminal TUI chat and flow client)
- impacted tech design: none directly (TUI client layer)
- impacted system spec: none directly

## 3. Environment and Reproduction

- environment: Windows 10.0.22631, Windows Terminal / Cursor terminal, `flowpilot chat`, provider `grok`.
- reproduction: start TUI, use normally. Hang appears with or without a flow; clicking the visible TUI often does not restore keys.
- frequency: multiple sessions on 2026-08-28; operator-confirmed live hangs at 09:42, 10:03, 10:19–10:29, 10:34–10:35.

## 4. Expected vs Actual

- expected: keys always reach `Update` when the terminal pane has host keyboard focus. Decision UI is usable with arrows + Enter. Click-to-focus the host pane is a real focus click, not an app chip click.
- actual: mouse tracking captures hover/click; keys (and many clicks) never enter the process; chips are the only convenient path for parked flow / approval / gate.

## 5. Impact

- users affected: Windows TUI operators first; keyboard ring is global.
- workflows affected: compose, parked flow, approval, question, gate, F2 agent open, attachments.
- severity: high.

## 6. Root Cause

- confirmed: `tuiProgramOpts` always sets `WithMouseCellMotion`. On Windows, Bubble Tea also sets `ENABLE_MOUSE_INPUT`. The host (WT / Cursor xterm) then reports mouse to the PTY without giving the pane keyboard focus. Same `ReadConsoleInput` queue can show `MOUSE_MOVED` (`motionLive=true`) while `KEY_EVENT` never appears. Operator “I already clicked” matches `target=""` clicks or clicks that never log.
- ruled out: BUG-325 gate swallow, paste flood, Alt+V hang, renderer block, dead reader (event loop + motion/click fingerprints).
- not chosen: Win32 `GetConsoleWindow` vs `GetForegroundWindow` (NULL/hidden on ConPTY; false `foreground=false` always). In-place `tea.Program` restart (does not create host key events; double-`Init`).

## 7. Fix Strategy

### F-1 Stop mouse tracking

- `tuiProgramOpts`: `{tea.WithAltScreen(), tea.WithFilter(tuiMsgFilter)}` only.
- `pulseMouseTracking` stays a no-op.
- Wheel: transcript scroll is already **PgUp/PgDn**. Native host wheel on alt-screen is accepted loss (same as Claude Code with mouse fully off).
- Drag-select inside the app goes away; **native host selection** replaces `[copy]` / in-app drag (operator accepted).

### F-2 Action ring (chips stay visible, keys drive them)

One ordered list of actions derived from the same labels `clickTargetAt` already knows. Highlight with inverse / `styleStatusHi` (same idea as `modeSetupModalPickerIdx`).

**When the ring is active** (exactly one of: `flowLoopBlocked()`, `approval != nil`, `question != nil`, `gate != nil`, unresolved attention):

| Key | Binding |
|---|---|
| `Left` / `Right` | Cycle highlight. Does **not** move the composer caret while the ring is active. |
| `Enter` | See Enter rules below. |
| `Esc` | If `gate.AwaitingCustom` → cancel custom, keep gate. Else if `actionRingFocus` → set it false (Left/Right return to caret; card stays). Else existing Esc (attach panel, selection, child back, clear `/`). |
| `1`–`9` | When input is empty: activate nth **visible** action immediately. When input is non-empty: insert the digit (composer). |
| `Tab` / `Up` / `Down` | **Unchanged** (picker / agents / history). Ring does not steal them. |

Default highlight: first rendered action (Retry, Approve, first question option, first gate option). Stop is never default.

`actionRingFocus`: starts **true** whenever a must-answer card is armed (approval, question, gate, attention). Starts **false** for parked-blocked only; the first `Left`/`Right` sets it true. Esc clears it.

**Enter rules (in order):**

1. Slash picker open → existing picker.
2. Input is a slash command → existing `processInput`.
3. `gate.AwaitingCustom` and input non-empty → submit custom (existing).
4. Must-answer card (approval / question / gate / attention) and input empty → activate highlight.
5. Parked blocked and (`actionRingFocus` or input empty) → activate highlight (default Retry). Typed non-slash chat + Enter still sends (run-136749: composer stays enabled).
6. Else → existing Enter.

### F-3 Map by chip group

#### A. Status / chrome

| Surface | Keyboard |
|---|---|
| `[info]` / session / `[collapse]` | **F2** (unchanged) |
| `skills:N` | **F3** |
| status details | **F4** |
| `[stop]` on a **live** turn | **Ctrl+C** (already `cmdStopTurn`) or `/stop` |
| `[stop]` on status when parked | not this chip; use ring `[Stop]` (F-3 C) |

Remove dependence on clicking the status `[stop]` chip.

#### B. F2 `[open]` / `[back]`

| Surface | Keyboard |
|---|---|
| Open child transcript | **Tab** / **Shift+Tab** when no slash picker: existing `cmdFocusAgent` cycle (this **is** `[open]`). |
| Back to main | Cycle Tab until main, or **Esc** while `viewingChild()` (already in child-view key allow-list — keep `/agent main`). |
| Pick a specific step in F2 without cycling all agents | While F2 panel is expanded and slash picker is closed: **`[` / `]`** move highlight among steps that have a child run; **`o`** opens highlighted step (`cmdFocusAgent`). Does not steal Enter/Tab/arrows. |

`[` `]` / `o` are no-ops if F2 is collapsed (operator hits F2 first).

#### C. Parked flow `[Retry]` `[Allow]` `[Stop]` (`[Continue]` = Retry alias)

Ring actions in render order, skipping hidden Allow (cap / member_stalled / no drift — same as `hitBlockedChrome`).

| Highlight | Same as today’s click |
|---|---|
| `[Retry]` / `[Continue]` | `cmdContinueFlow` |
| `[Allow]` | `cmdAmendFlow` when visible |
| `[Stop]` | `cmdStopTurn` (parked path already in mouse + `/stop`) |

Hint on the blocked bar: `← → select · Enter · /continue /stop`.

#### D. Approval / question / gate / attention

| Surface | Ring actions | Notes |
|---|---|---|
| Approval | Approve, Deny, Approve all, Deny all, Approve forever, runner `adec:` labels | `/approve` `/deny` remain |
| Question | Each `N)` / label; Submit if multi-select | **Space** toggles `qtoggle` when multi-select and ring focused; Enter submits (`qsubmit` or single `qopt`) |
| Gate | `[Fix code]` `[Suggest req]` `[Custom]` / other `gopt:` | Custom + Enter (empty) arms `AwaitingCustom`; then typing + Enter submits custom (existing) |
| Attention | `[inspect]` `[confirm-retry]` resolve/repair chips in render order | Two-step confirm-retry stays: first Enter arms, second Enter retries |

#### E. Images

| Surface | Keyboard |
|---|---|
| Paste | **Alt+V** / `/image paste` (unchanged) |
| Open manage panel | **`/image`** with no args **and** `len(pendingAttach)>0` opens the panel (today it only prints a tip). Optional: same as former `[N img]` click. |
| Inside panel | **Up/Down** select row (overrides prompt-history while panel open). **Enter** = `attach-open`. **`x` or `d`** = `attach-rm`. **Esc** closes (already). |
| `/image open n` / `/image rm n` | Remain as fallback |

#### F. Copy / transcript extras

| Surface | Keyboard |
|---|---|
| `[copy]` / `copyfence` | Dropped. Native select+copy. `/copy` remains. |
| You-box expand | No click. Optional later: not in this bug. Clamped box stays readable; `/copy` for full text. |
| `tool-group` expand | No click. Groups stay collapsed/expanded as produced; optional `[` `]` not used here. Out of scope unless a test requires it — **defer tool-group click; keyboard collapse is not required to unblock hang.** |
| `load-earlier` | Keep existing scroll / command path if any; no new key required this bug. |

### F-4 Key dispatch priority

First match wins, documented so `handleKey` stays deterministic:

1. Auth modal / `/mode-setup` modal (unchanged).
2. Attach panel open → image keys (F-3 E).
3. `collectSuggestions() > 0` → existing picker (Tab/Up/Down/Enter).
4. `actionRingFocus` or (empty input + (approval \| question \| gate \| parked blocked \| attention)) → ring Left/Right/Enter/1–9 as in F-2.
5. F2 expanded + `[` `]` `o`.
6. Default: composer, Tab agent/posture, Up/Down history, PgUp/PgDown scroll.

### F-5 Watchdog (small, same bug)

- Fingerprint: do not arm `lastInputAt` from the first watchdog tick; leave zero until a real `KeyMsg` (or log `hadRealKey=false`).
- Do not add Win32 foreground.
- Banner copy: stop saying “press any key or Ctrl+C to restart” as the only hint; idle false-positive remains eligibility-gated as today.

### Non-goals

- No mouse re-enable flag in this bug (can be a later opt-in).
- No Job Object / forked coninput reader.
- No hydrate-retry infinite loop (separate bug, section 10).

## 8. Validation

- `V-1` `tuiProgramOpts` has no `WithMouseCellMotion` (extend `tui_program_opts_test.go` with a **new** test file if the existing test must stay untouched — prefer new `bug328_program_opts_test.go`).
- `V-2` Parked flow: empty composer, Right to `[Stop]`, Enter → stop command; default empty Enter → Retry/continue. `/continue` still works.
- `V-3` Approval/question/gate: Left/Right highlight, Enter dispatches same commands as today’s clicks. `1`–`9` on empty input.
- `V-4` Tab with agents and no picker still `cmdFocusAgent`. `[` `]` `o` opens a mapped F2 child without Tab.
- `V-5` `/image` with pending images opens panel; Up/Down/Enter/`x`/Esc work; prompt history Up/Down does not fire while panel open.
- `V-6` Live turn Ctrl+C still stops; Ctrl+C idle+empty still quits; parked Stop is not Ctrl+C.
- `V-7` `go test ./internal/tui/app/... -count=1` green; pre-existing tests untouched except if a test **asserts** `WithMouseCellMotion` — then add a new test and only change production opts; if an old test fails solely because mouse is gone, **stop and ask** (additive-tests-only).
- `V-8` Manual Windows: click the terminal chrome/pane, type immediately; hover must not be required. Native select copies text.

## 9. Regression Guard

- tests (new files):
  - `bug328_program_opts_test.go` — no cell-motion option.
  - `bug328_action_ring_test.go` — blocked Retry/Allow/Stop, approval, gate, question; Enter vs slash vs typed chat; Ctrl+C vs parked Stop.
  - `bug328_f2_open_key_test.go` — `[` `]` `o` and Tab agent cycle still work.
  - `bug328_image_panel_keys_test.go` — `/image` opens panel; x/Enter/Esc.
- alerts: watchdog fingerprint may add `hadRealKey=`.
- audit: CA after implement; will-not-undo list in Constraints.

## 10. Follow-Up Document Updates

- CP-56: TUI is keyboard-primary; mouse cell-motion is off; action ring + F2 `[` `]` `o`.
- Help text / blocked bar / `/help`: print the map in F-2 / F-3.
- Hydrate-retry infinite loop (pid `21436`) stays a separate BUG.
- `Q-2` watchdog test inverted assertion stays unfixed unless approved.
