# CA-705 — TUI first-turn blank + Tab busy guard (BUG-341 / B4)

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: BUG-341
change_type: bugfix
summary: fix first-turn blank (tool-only turnStream closed before turn_completed) and Tab scan→plan 409 + in-place apply leak via turnLive busy flag and C2 guard
# --->8---

## What changed

- `model.go`: new `turnLive bool` — true from local send (`processInput`) until `turn_completed`/`turn_failed` via any stream (`handleEvent`, `TurnDoneMsg`, `turnFinishedMsg`, `TurnFailedMsg`). Survives stream switch (`turnStream` → `orchStream`).
- `app.go` `processInput`: sets `turnLive = true` on both startRun and sendTurn paths.
- `app.go` `handleEvent` `turn_completed`/`turn_failed` and `TurnDoneMsg`/`turnFinishedMsg`/`TurnFailedMsg`: clear `turnLive = false`.
- `app.go` `turnStreamClosedMsg`: on `Err != nil` clear `turnLive`; on `Err == nil` but `turnLive == true` keep `ConnRunning`/`turn running…` (or `thinking…` if placeholder present), start `orchStream`, do not settle to `done` nor emit `no assistant text`. The blank on 417944 (6 tool calls, no delta, orch late) now waits for `orch turn_completed` which renders via `handleEvent`.
- `chat_switch.go` `routeProviderSwitch` / `routePostureSwitch` C2: add `m.turnLive ||` to the busy check (`turnStream`/`turnSendPending`/`question`/`approval` already there). Tab while live now returns C2 `detachedNoticeMsg` instead of attempting the switch endpoint (which would 409 `handoff_run_busy`).
- `chat_posture.go` `chatPostureCmdFromPending` `apply:` branch: after `routePostureSwitch` returns nil, check `chatSwitchInFlight || turnLive || turnStream != nil || turnSendPending || question/approval` — if busy, return C2 notice instead of falling through to `applyChatPostureProfile` in-place. Fixes the double-Tab leak where scan→plan failed with 409 but still applied `plan`'s grok provider onto the opencode leg, requiring a second Tab to reach `code` (417944→417970).

## R1 evidence

- New tests, no old-test edits (additive-tests-only):
  - `bug341_turnlive_blank_test.go` (4 tests): `TestFirstTurnStreamClosedKeepsLiveUntilTerminal`, `TestFirstTurnOrchTurnCompletedRendersReply`, `TestFirstTurnWithThinkingStaysLive`, `TestFirstTurnBlankIsProviderAgnostic` (table `grok/codex/claude/opencode`).
  - `bug342_tab_busy_test.go` (6 tests): `TestTabWhileTurnLiveBlocked`, `TestTabPendingWhileInFlightDoesNotApplyInPlace`, `TestDoubleTabWhileInFlightOnlyOneLeg`, `TestSameProviderTabWhileTurnLiveBlocked`, `TestTabBusyIsProviderAgnostic` (table over 4 providers), `TestTabAfterTurnCompletedWorks`.
- Old suite: `go test ./internal/tui/app -count=1` 7.06s PASS (0 failures, 0 edits). The existing `TestTurnStreamClosed_ClearsStrandedThinking` (run-96217) still passes because it runs with `turnLive==false` (the stranded-thinking case is not live).
- Provider parity: classification is provider-agnostic — the live flag and C2 guard are in TUI state, not per-adapter. Verified with tables over `grok/codex/claude/opencode` for both fixes (no providerKey branch). No per-provider adapter change, so shared-logic agnostic proof suffices.

## Honest gaps

- Desktop Chat vs Workflow last-surface persist and Desktop detached parity remain as in `CA-703`/`CA-704`.
- First-turn tool-only blank for workflow/flow runs uses the same `turnLive` path but was only live-tested on chat (opencode); flow provider parity is agnostic by the same flag.

## Prior CA not undone

- `CA-702` posture Active persist, `CA-699` detached reattach, `CA-697` posture Tab routing (`pinnedProviderFor`/`derive-once`), `CA-696` switch surface (`applyChatSwitched` keeps transcript, `addMessage` collapse) remain the source of truth. This CA extends their `ChatSwitchedMsg`/`routePostureSwitch` contracts, it does not replace them.
- `CA-533`/`CA-537` thinking placeholder semantics unchanged — the new `turnLive` path keeps the placeholder while live and only clears it after the terminal.
