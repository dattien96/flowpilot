# Task-311: TUI Reactive Sidebar Without F2/F4 Toggles

## Metadata

- Document ID: `Task-311`
- Title: `TUI Reactive Sidebar Without F2/F4 Toggles`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-28`
- Last Updated: `2026-08-28`
- Parent Documents: [CP-56: Terminal TUI Chat And Flow Client](../../07-Coding-Plan/done/CP-56-Terminal-TUI-Chat-And-Flow-Client.md), [BUG-328](../../09-BugFix/todo/BUG-328-TUI-Input-Dies-With-Zero-Key-Events-And-Cannot-Self-Recover.md)
- Child Documents: `None`
- Related Documents: [CA-670](../../../change-audit/CA-670-bug328-ss3-fkey-rebuild.md), [CA-671](../../../change-audit/CA-671-bug328-stall-notice-runner-job.md), [Task-309](../todo/Task-309-TUI-Desktop-Retry-Stop-Allow-For-Frozen-Contract-Drift.md)
- Replaces: `None`
- Tags: `cli-tui, sidebar, reactive-layout, keyboard-ux, windows`

## AI Quick View

### Summary

- The TUI's right sidebar is currently F2-gated (`sessionPanel.Collapsed` toggle) and the status details are F4-gated (`statusDetailsCollapsed` toggle). BUG-328 proved F-key spam is exactly what wedges Windows Terminal's keyboard dispatch — removing the toggles removes the trigger AND matches OpenCode's UX.
- **New behavior:** sidebar visibility is reactive to terminal width only (wide → sidebar shows, narrow → chat only). F4's hidden status details move into the sidebar. The main chat column keeps only the transcript, composer, and one always-visible status line.

### Current Ask

- Implement T-1…T-8. Additive tests only. F2/F4 no longer toggle anything. `/info` and slash commands unchanged.

### Key Decisions

- `T-D1` **Reactive width only.** `useRightSidebar()` = `hasContent && terminalWidth() >= tuiSidebarMinWidth` (new constant, default 110). The `!Collapsed` gate and the F2 toggle die.
- `T-D2` **F4 details live in the sidebar.** Mode / model / skills / account / context-limits rows render as sidebar sections instead of expandable status rows. `statusDetailsCollapsed` dies.
- `T-D3` **Main column = chat + one line.** `renderStatusLine` returns exactly the line-0 content (status label, flow name, toasts). The detail rows and the `[> F4]`/`[v F4]` fold chip are removed from the main column.
- `T-D4` **No overlay, no collapse chip.** `renderSessionPanelOverlay` usage is removed (panelLines only exist as the sidebar column); the `[collapse]` link in the sidebar is dropped.
- `T-D5` **F3 skills toggle follows F4.** Skills content moves into the sidebar section; `statusSkillsExpanded` is removed from the main line. F3 key becomes a no-op (kept from stealing anything).
- `T-D6` Slash commands and `/info` remain as fallback text paths.

### Constraints

- Additive-tests-only: do not edit green pre-existing tests that assert F2/F4 toggling or the expanded status rows unless they ONLY test the removed behavior — then stop and ask (`Q-1`).
- Must not undo BUG-328 input work (single record reader, no F-key dependency, watchdog notice, runner Job Object).
- Keyboard contract on Windows/macOS/Linux identical — no `ProviderKey` branch.
- Width behavior must be deterministic from `WindowSizeMsg` only.

### Open Questions

- `Q-1` Several green tests assert `F2`/`F4` toggle semantics (`status_bar_layout_test.go`, `bug328_f2_f4_status_test.go`, `session panel` tests). Flipping production without editing them requires the notices to keep firing — list exact tests during implementation and ask before editing any.
- `Q-2` Sidebar width threshold: 110 proposed (42-col sidebar leaves ≥ 67 for chat). Operator may prefer OpenCode's ~120.

### Source Refs

- `session_panel.go` — `useRightSidebar`/`sideWidth`/`renderRightSidebar`/`renderSessionPanelOverlay`/`sessionInfoPanel`.
- `status_bar.go` — `renderStatusLine`/`renderStatusLine0`/`statusFoldChip`/`statusReadyLabel`/`statusNoticeBase`/detail-row renderers.
- `app.go` — F2/F3/F4 handlers, `tuiChrome` (`panelLines`/`sideActive`), `View` compose cache.
- OpenCode UX reference: sidebar follows width, no toggle key.

## 1. Goal

Remove F2/F4 keyboard toggles from the TUI. The right sidebar becomes a pure width-reactive column holding session info, flow steps, and all status details. The main column is transcript + composer + one always-visible status line. Rationale: (a) BUG-328 shows F-key bursts trigger the Windows Terminal input wedge; (b) OpenCode-style reactive layout is the requested UX.

## 2. Parent Links

- coding plan: [CP-56](../../07-Coding-Plan/done/CP-56-Terminal-TUI-Chat-And-Flow-Client.md)
- tech design: none directly (TUI client layer)
- system spec: none directly
- bugfix: [BUG-328](../../09-BugFix/todo/BUG-328-TUI-Input-Dies-With-Zero-Key-Events-And-Cannot-Self-Recover.md) (F-3 A: F2/F4 status toggles are superseded)

## 3. Trigger

Operator UX decision after BUG-328: stop depending on F2/F4 entirely; sidebar reacts to window width like OpenCode.

## 4. Exact Change

- `T-1` **Reactive sidebar gate.** `useRightSidebar()`: `hasContent() && terminalWidth() >= tuiSidebarMinWidth`. Delete `sessionPanel.Collapsed` reads in the render path (field may stay for state-migration or be removed with its setters). F2 handler: remove the toggle + its `statusNoticeBase` message; F2 becomes a no-op (or `/info` alias — operator decides, default no-op).
- `T-2` **F4 details into the sidebar.** Move mode / model / skills / account / context-limits rows from `renderStatusLine` detail rows into `renderRightSidebar` as sections after steps. `statusDetailsCollapsed` deleted; `statusFoldChip()` deleted.
- `T-3` **One-line main status.** `renderStatusLine` returns only line 0 content: `statusReadyLabel` (+ flow name / toasts as today). Delete the multi-row assembly and the `[collapse]`/`[info]` chips. `statusNoticeBase` stays (F2/F4 notices removed; other notices keep "ready · ..." composition).
- `T-4` **Overlay removal.** `tuiChrome.panelLines` no longer comes from `renderSessionPanelOverlay`; when the sidebar is inactive the panel lines are nil (main column untouched). `renderSessionPanelOverlay` deleted or reduced to the sidebar path. `[collapse]` sidebar link deleted.
- `T-5` **F3 no-op.** `statusSkillsExpanded` removed from the main status; skills render in the sidebar section; F3 key becomes a no-op.
- `T-6` **Hit-test / chrome.** `mouse.go tuiChrome` panelHeight/statusY math updated for the one-line status (statusH always 1). `clickTargetAt` unchanged (no panel chips to hit).
- `T-7` **New tests** (`task311_reactive_sidebar_test.go`): width ≥ threshold → sidebar present (`useRightSidebar` true, `contentWidth` = width − sideWidth − 1); narrow → false, chatWidth = terminalWidth; F2/F4 KeyMsg are no-ops (state unchanged, no notices); F3 no-op; `renderStatusLine` never contains `\n` beyond the toast join; sidebar contains mode/model/account/limits sections.
- `T-8` **Width threshold constant** `tuiSidebarMinWidth = 110` (see `Q-2`).

## 5. Touched Areas

- files: `session_panel.go`, `status_bar.go`, `app.go` (F2/F3/F4 handlers, `View`, `tuiChrome` consumers), `mouse.go` (chrome math), `task311_reactive_sidebar_test.go`, this task doc, CA-672
- modules: `apps/local-runner/internal/tui/app`
- routes: none
- tables: none

## 6. Acceptance Check

- New T-7 tests green; full `go test ./internal/tui/app/ -count=1` green (after `Q-1` review).
- Manual (Windows + macOS parity): drag terminal wide → sidebar appears with session/steps/details; narrow → sidebar gone, main column is chat + one status line; pressing F2/F4/F3 does nothing visible.
- No `renderSessionPanelOverlay` call sites remain in the chat-pane path.
- `pendingConsoleEvents` / watchdog behavior unchanged (BUG-328 intact).

## 7. Out of Scope

- Slash-command keyboard paths (`/info`, `/image`, …).
- Action ring / gate / approval / question UX.
- Mouse chip hit-testing for the sidebar.
- Any Android/Desktop parity work.

## 8. Completion Notes

- result: pending
- follow-ups: pending
- upstream docs updated: pending