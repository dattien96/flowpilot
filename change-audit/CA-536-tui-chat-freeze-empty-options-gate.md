---
id: CA-536
feature_key: cli-tui
title: Fix TUI chat freeze after empty-options flow gate (run-103672)
date: 2026-08-17
status: COMPLETE
---

## Problem

run-103672: after a flow gate trigger the TUI printed "Gate options:
(enter number or name)" with **no options** and the operator could not chat
again. The chat box stayed visible (`[chat]`) and typing still worked, but
every Enter was swallowed as a gate decision that never matched, re-printing
the empty prompt (the report shows it three times).

Root cause chain (verified in code):

1. The runner **always** emits `flow_gate_violation` for every gate verdict
   (`gate_hook.go:389`, `:1111`) — not just real blocks:
   - `result.Action` is `warn` in warn gate-mode (Enforce downgrades
     block/reprompt to warn) or `reprompt` for the auto fix-code re-drive
     (`emitStatus = "reprompt"`, `applyGateFixCodeAutoRepromptLocked`).
   - `GateOptions` is only populated from an `r-reg` violation with options
     (`gate_hook.go:362-370`); every other rule / the auto-reprompt path
     (`emitOptions = nil`) emits **empty** `GateOptions`.
   - `gate_blind_hook.go:44` and the commitChangeContract fail-closed site
     (`gate_hook.go:337`) also emit with no options at all.
2. The event carries the verdict in `status` (`provider_event.go:134`), but the
   TUI client's `ProviderEvent` never parsed it — the field was dropped in
   `client.go`.
3. `handleEvent` armed `m.gate` unconditionally (`app.go:1342-1350`), so an
   empty-options event set `m.gate` with `Options=nil`.
4. `processInput` prefers the gate over chat (`app.go:2045`):
   `if m.gate != nil { return m.handleGateInput(input) }`.
5. `handleGateInput` only accepts an option number/name and otherwise re-prints
   "Gate options:  (enter number or name)". With zero options **nothing could
   ever match** → the composer was locked forever.
6. `turn_completed` / `TurnDoneMsg` / `turnFinishedMsg` never cleared `m.gate`,
   so even when the flow finished (requirements filled, tests passing) the lock
   persisted. The only exit was `/new`.

## Fix (TUI-only, additive)

1. **TUI client parses `status`** — `client.go` `ProviderEvent` gains
   `Status string json:"status,omitempty"` (already present in the SSE JSON from
   the runner's twin struct, so live + replayed events both carry it).
2. **Arm the gate only for a real decision card** — `handleEvent`
   `flow_gate_violation` now gates on
   `strings.EqualFold(ev.Status, "block") && len(ev.GateOptions) > 0`.
   A genuine block with r-reg options still arms `m.gate`, sets
   `ConnWaiting`/"gate", and shows the card (CA-534 preserved). Every other
   verdict (warn, reprompt, block-without-options) just surfaces the
   `buildGateMessage` info message and does **not** lock the composer — the flow
   runner itself stays authoritative for warn/reprompt.
3. **Escape hatch** — `handleGateInput` first checks
   `m.gate == nil || len(m.gate.Options) == 0` and clears the gate, so any
   option-less armed gate (legacy replay, race) is dropped instead of looping.
4. **Clear stale gates on turn completion** — `turn_completed` (live),
   `TurnDoneMsg` and `turnFinishedMsg` now set `m.gate = nil`. Once the flow
   moved on, a still-armed gate can never be legitimate.

## Provider impact

Provider-agnostic (Case 1). The verdict/options handling is run-level, not
adapter logic. All matrix tests parameterize Claude / Codex / Grok.

## Tests

New additive file `tui/app/ca536_gate_empty_options_test.go`:

- `TestGateViolation_WarnStatusDoesNotArmGate` — warn verdict must not arm the
  gate or set `statusMsg=gate`.
- `TestGateViolation_RepromptStatusDoesNotArmGate` — reprompt (auto fix-code
  re-drive) must not arm the gate.
- `TestGateViolation_BlockWithoutOptionsDoesNotArmGate` — block without a
  decision card must not arm the gate.
- `TestGateViolation_BlockWithOptionsStillArmsGate` — a real block with
  `continue/stop` still arms the gate + `statusMsg=gate` (CA-534 contract).
- `TestGateViolation_EmptyOptionsChatStillSends` — after an empty-options
  verdict a freeform input returns a non-nil send cmd, the gate stays clear,
  and "Gate options:" never reprints.
- `TestGateInput_EmptyOptionsEscapeHatch` — an option-less armed gate is cleared
  by the escape hatch with no gate-submit cmd.

All matrix tests run per provider (`claude`/`codex`/`grok`).

## Verification

- `go build ./...` clean; `go vet ./internal/tui/...` clean.
- gofmt clean; new test file written CRLF (repo-wide convention).
- `go test ./internal/tui/app/ -count=1` green (excluding the two known
  pre-existing flaky `TestCmdFocusAgent_*` network tests — re-confirmed to fail
  on the clean baseline with changes stashed).
- `go test ./internal/tui/client/ -count=1` green.
- Legacy gate tests untouched and green: `tui_run_sidebar_gate_paste_history_test.go`,
  `app_extended_test.go` (`TestA7_11`/`TestA9_4` still assert the info message
  text in the view), `chat_windowing_test.go`.

## Out of scope / residual

- The runner's emit behavior is intentionally unchanged — Desktop relies on the
  event for warn/reprompt too; the TUI now just refuses to lock on it.
- Headless mode (`app.go:4261`) still treats any `flow_gate_violation` as fatal;
  unchanged (no composer to freeze).
- `pendingGateBlock` on the runner side still requires `len(gateOptions) > 0`,
  consistent with the TUI arming rule.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CA-536
change_type: bugfix
summary: Fix TUI chat freeze after a flow gate with empty options (run-103672) by parsing the runner verdict status, arming the interactive gate only for a real block with a decision card, adding an escape hatch for option-less gates, and clearing stale gates on turn completion
# --->8---